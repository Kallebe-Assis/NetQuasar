import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { RefreshCw } from "lucide-react";
import { ConfirmModal } from "../components/ConfirmModal";
import { DashboardPageLoader } from "../components/DashboardPageLoader";
import { InfoHint } from "../components/InfoHint";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
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

const CHART_COLORS = ["#58a6ff", "#3fb950", "#d29922", "#f85149", "#a371f7", "#79c0ff", "#ff7b72", "#56d364", "#ffa657"];

type TopRow = { device_id: string; description: string; ip?: string | null; latency_ms: number };

type DashboardTotals = {
  devices?: number;
  pops?: number;
  commercial_clients_sum?: number;
  monitoring_running?: boolean;
  telemetry_enabled_devices?: number;
  ping_enabled_devices?: number;
};

type NamedCount = { category?: string; network_status?: string; operational_mode?: string; count?: number; pop_name?: string; locality_name?: string; alert_type?: string };

type LatRank = { device_id?: string; description?: string; avg_latency_ms?: number; samples?: number };
type MikTraffic = { device_id?: string; description?: string; if_in_octets?: number | null; if_out_octets?: number | null; collected_at?: string };
type OltOnu = {
  device_id?: string;
  description?: string;
  onu_count?: number;
  onu_online?: number;
  onu_offline?: number;
  brand?: string;
  snapshot_at?: string;
};

type DashboardAnalytics = {
  generated_at?: string;
  days?: number;
  since?: string;
  totals?: DashboardTotals;
  devices_by_category?: NamedCount[];
  devices_by_network_status?: NamedCount[];
  devices_by_operational_mode?: NamedCount[];
  devices_by_pop?: Array<{ pop_id?: string; pop_name?: string; count?: number }>;
  devices_by_locality?: Array<{ locality_id?: string; locality_name?: string; count?: number }>;
  ping_ranking_worst_latency?: LatRank[];
  ping_ranking_best_latency?: LatRank[];
  telemetry_window?: { samples?: number };
  alerts_by_type_30d?: NamedCount[];
  alerts_open?: number;
  mikrotik_interface_traffic_latest?: MikTraffic[];
  olt_onu_by_device?: OltOnu[];
  olt_onu_fleet_totals?: { onu_count?: number; onu_online?: number; onu_offline?: number };
};
type OltCapacityPON = {
  olt_id: string;
  olt: string;
  pon_id: string;
  onu_total: number;
  usage_percent: number;
  near_saturation: boolean;
};
type OltCapacity = {
  olt_rows: Array<{ olt_id: string; olt: string; onu_total: number; pon_count: number; near_saturation_pons: number; snapshot_at: string }>;
  pon_rows: OltCapacityPON[];
  trend_7d: Array<{ day: string; onu_total: number }>;
};

function num(n: unknown): number {
  const x = Number(n);
  return Number.isFinite(x) ? x : 0;
}

function fmtInt(n: unknown): string {
  const x = num(n);
  return new Intl.NumberFormat("pt-PT").format(Math.round(x));
}

function fmt1(n: unknown): string {
  const x = num(n);
  return Number.isFinite(x) ? x.toFixed(1) : "—";
}

function fmtBytes(n: unknown): string {
  const x = num(n);
  if (x <= 0) return "—";
  const u = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = x;
  let i = 0;
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : Math.round(v)} ${u[i]}`;
}

function trunc(s: string, max = 22): string {
  const t = String(s ?? "").trim();
  return t.length <= max ? t : `${t.slice(0, max - 1)}…`;
}

const tooltipStyle = {
  backgroundColor: "var(--panel2)",
  border: "1px solid var(--border)",
  borderRadius: "var(--radius)",
  fontSize: 12,
};

function Section({
  id,
  title,
  subtitle,
  children,
}: {
  id: string;
  title: string;
  subtitle?: string;
  children: React.ReactNode;
}) {
  return (
    <section id={id} className="card" style={{ marginTop: 16, padding: "14px 16px 18px" }}>
      <h2
        style={{
          marginBottom: 12,
          color: "var(--text)",
          textTransform: "none",
          letterSpacing: 0,
          display: "flex",
          alignItems: "center",
          gap: 6,
          flexWrap: "wrap",
        }}
      >
        {title}
        {subtitle ? <InfoHint label={title}>{subtitle}</InfoHint> : null}
      </h2>
      {children}
    </section>
  );
}

function ChartBox({ h, children }: { h: number; children: React.ReactElement }) {
  return (
    <div style={{ width: "100%", height: h, minHeight: h }}>
      <ResponsiveContainer width="100%" height="100%">
        {children}
      </ResponsiveContainer>
    </div>
  );
}

export function DashboardPage() {
  const qc = useQueryClient();
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
  });

  const topProbe = useQuery({
    queryKey: dashboardTopLatencyKey,
    queryFn: wrapPageCachedQueryFn(dashboardTopLatencyKey, () =>
      apiFetch<{ top: TopRow[] }>("/api/v1/overview/top-latency?limit=8"),
    ),
    ...pageCachedQueryOptions<{ top: TopRow[] }>(dashboardTopLatencyKey, DASHBOARD_STALE_MS, DASHBOARD_GC_MS),
  });
  const cap = useQuery({
    queryKey: dashboardOltCapacityKey,
    queryFn: wrapPageCachedQueryFn(dashboardOltCapacityKey, () => apiFetch<OltCapacity>("/api/v1/dashboard/olt-capacity")),
    ...pageCachedQueryOptions<OltCapacity>(dashboardOltCapacityKey, DASHBOARD_STALE_MS, DASHBOARD_GC_MS),
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

  const mikTrafficBar = useMemo(() => {
    return (dash.data?.mikrotik_interface_traffic_latest ?? []).map((r) => {
      const inn = r.if_in_octets != null ? num(r.if_in_octets) : 0;
      const out = r.if_out_octets != null ? num(r.if_out_octets) : 0;
      return {
        name: trunc(String(r.description ?? r.device_id ?? "?"), 18),
        "Entrada (GiB)": inn / 1024 ** 3,
        "Saída (GiB)": out / 1024 ** 3,
        _rawIn: inn,
        _rawOut: out,
      };
    });
  }, [dash.data?.mikrotik_interface_traffic_latest]);

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
    if (dash.isLoading && !dash.data) {
      setPageIn(false);
      return;
    }
    if (dash.data && !dash.isError) {
      const id = requestAnimationFrame(() => setPageIn(true));
      return () => cancelAnimationFrame(id);
    }
    setPageIn(false);
  }, [dash.isLoading, dash.data, dash.isError]);

  const initialLoading = dash.isLoading && !dash.data;
  const isFetching = dash.isFetching || topProbe.isFetching || cap.isFetching || refreshing;

  if (initialLoading) return <DashboardPageLoader />;
  if (dash.isError) return <div className="msg msg--err">{(dash.error as Error).message}</div>;

  return (
    <div className={`dashboard-page${pageIn ? " dashboard-page--in" : ""}`}>
      <ConfirmModal
        open={refreshConfirmOpen}
        title="Actualizar dashboard"
        message="Vai recarregar todos os gráficos e indicadores do dashboard. Esta operação pode demorar alguns segundos, conforme o volume de dados no servidor."
        confirmLabel="Actualizar agora"
        cancelLabel="Cancelar"
        busy={refreshing}
        onCancel={() => !refreshing && setRefreshConfirmOpen(false)}
        onConfirm={() => void runFullRefresh()}
      />
      <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 10 }}>
        <h1 style={{ margin: 0 }}>
        Dashboard analítico
        <InfoHint label="Sobre o dashboard analítico">
          <p>
            Visão agregada dos últimos <strong>{dash.data?.days ?? days}</strong> dias. Métricas de coleta (ping, telemetria, tráfego, ONUs)
            consideram apenas equipamentos em operação <strong>Ativo</strong>.
          </p>
        </InfoHint>
        </h1>
        <button
          type="button"
          className="btn"
          disabled={refreshing}
          onClick={() => setRefreshConfirmOpen(true)}
          title="Recarregar todos os dados do dashboard"
        >
          <RefreshCw size={14} style={{ marginRight: 6, verticalAlign: -2 }} />
          {refreshing ? "A actualizar…" : "Actualizar dados"}
        </button>
      </div>

      <div className="row" style={{ flexWrap: "wrap", gap: 10, alignItems: "center", marginBottom: 8 }}>
        <label style={{ fontSize: 12, color: "var(--muted)" }}>
          Período (dias)
          <select className="select" style={{ marginLeft: 8, minWidth: 100 }} value={days} onChange={(e) => setDays(Number(e.target.value) || 30)} title="Janela temporal das séries">
            {[7, 14, 30, 60, 90].map((d) => (
              <option key={d} value={d}>
                {d} dias
              </option>
            ))}
          </select>
        </label>
        <span className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
          Gerado: {dash.data?.generated_at ? new Date(dash.data.generated_at).toLocaleString("pt-PT") : "—"}
          {isFetching ? " · a actualizar…" : " · em cache"}
        </span>
      </div>

      <div className="dashboard-kpi-row">
        <div className="stat">
          <div className="stat__k">Equipamentos</div>
          <div className="stat__v">{fmtInt(totals?.devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">POPs</div>
          <div className="stat__v">{fmtInt(totals?.pops)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Comercial (clientes, mês actual UTC)</div>
          <div className="stat__v">{fmtInt(totals?.commercial_clients_sum)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Ping ligado</div>
          <div className="stat__v">{fmtInt(totals?.ping_enabled_devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Telemetria ligada</div>
          <div className="stat__v">{fmtInt(totals?.telemetry_enabled_devices)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Monitorização</div>
          <div className="stat__v" style={{ fontSize: 14 }}>
            {totals?.monitoring_running ? <span className="badge badge--ok">A correr</span> : <span className="badge badge--off">Parada</span>}
          </div>
        </div>
        <div className="stat">
          <div className="stat__k">Amostras telemetria</div>
          <div className="stat__v">{fmtInt(dash.data?.telemetry_window?.samples)}</div>
        </div>
        <div className="stat">
          <div className="stat__k">Alertas abertos</div>
          <div className="stat__v">{fmtInt(dash.data?.alerts_open)}</div>
        </div>
      </div>

      <Section
        id="sec-inventario"
        title="1 · Inventário e estado operacional"
        subtitle="Distribuição por categoria, estado de rede (Normal/Bridge) e modo operacional (Ativo, Manutenção, etc.). Escolha torta ou barras para categorias."
      >
        <div className="row" style={{ flexWrap: "wrap", gap: 8, marginBottom: 10 }}>
          <span style={{ fontSize: 12, color: "var(--muted)" }}>Visualização categorias:</span>
          <button type="button" className={catViz === "pie" ? "btn btn--primary" : "btn"} onClick={() => setCatViz("pie")}>
            Torta
          </button>
          <button type="button" className={catViz === "bar" ? "btn btn--primary" : "btn"} onClick={() => setCatViz("bar")}>
            Barras
          </button>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: 16 }}>
          {catViz === "pie" ? (
            <ChartBox h={300}>
              <PieChart margin={{ top: 8, right: 8, bottom: 48, left: 8 }}>
                <Pie data={catPie} dataKey="value" nameKey="name" cx="50%" cy="42%" outerRadius={78} label={false}>
                  {catPie.map((_, i) => (
                    <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Legend
                  layout="horizontal"
                  verticalAlign="bottom"
                  align="center"
                  wrapperStyle={{ fontSize: 11, lineHeight: 1.35, paddingTop: 8 }}
                />
              </PieChart>
            </ChartBox>
          ) : (
            <ChartBox h={280}>
              <BarChart data={catBar} margin={{ left: 4, right: 8, top: 8, bottom: 40 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 10 }} interval={0} angle={-25} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--accent)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          )}
          <ChartBox h={280}>
            <PieChart>
              <Pie data={netDonut} dataKey="value" nameKey="name" innerRadius={55} outerRadius={100} paddingAngle={2}>
                {netDonut.map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[(i + 2) % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Legend />
            </PieChart>
          </ChartBox>
          <ChartBox h={280}>
            <BarChart data={opModeBar} layout="vertical" margin={{ left: 8, right: 16, top: 8, bottom: 8 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" horizontal={false} />
              <XAxis type="number" tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
              <YAxis type="category" dataKey="name" width={100} tick={{ fill: "var(--muted)", fontSize: 10 }} />
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Bar dataKey="Quantidade" fill="#a371f7" radius={[0, 4, 4, 0]} />
            </BarChart>
          </ChartBox>
        </div>
      </Section>

      <Section
        id="sec-localidades"
        title="2 · POPs e localidades comerciais"
        subtitle="Quantidade de equipamentos associados a cada POP e a cada localidade cadastrada no módulo comercial (inclui zeros à esquerda para referência)."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))", gap: 16 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Por POP</h3>
            <ChartBox h={300}>
              <BarChart data={popBar} margin={{ left: 0, right: 8, top: 8, bottom: 48 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-30} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--ok)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Por localidade</h3>
            <ChartBox h={300}>
              <BarChart data={locBar} margin={{ left: 0, right: 8, top: 8, bottom: 48 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-30} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="Equipamentos" fill="var(--warn)" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          </div>
        </div>
      </Section>

      <Section
        id="sec-rankings"
        title="3 · Rankings de latência (média na janela)"
        subtitle="Apenas equipamentos Ativos com pelo menos 3 amostras válidas na janela. À direita, última latência na cache de sondas."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 14 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Piores médias (ms)</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Equipamento</th>
                    <th className="mono">Média</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.ping_ranking_worst_latency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Melhores médias (ms)</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>#</th>
                    <th>Equipamento</th>
                    <th className="mono">Média</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.ping_ranking_best_latency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Top latência (cache actual)</h3>
            {topProbe.isLoading ? (
              <p style={{ fontSize: 12 }}>…</p>
            ) : topProbe.isError ? (
              <div className="msg msg--err" style={{ fontSize: 11 }}>
                {(topProbe.error as Error).message}
              </div>
            ) : (
              <div className="table-wrap">
                <table style={{ fontSize: 11 }}>
                  <thead>
                    <tr>
                      <th>Equipamento</th>
                      <th className="mono">ms</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(topProbe.data?.top ?? []).map((r) => (
                      <tr key={r.device_id}>
                        <td>{trunc(r.description, 24)}</td>
                        <td className="mono">{r.latency_ms}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
        <ChartBox h={260}>
          <BarChart
            data={(dash.data?.ping_ranking_worst_latency ?? []).map((r) => ({
              name: trunc(String(r.description ?? ""), 12),
              "Latência média (ms)": num(r.avg_latency_ms),
            }))}
            margin={{ left: 8, right: 8, top: 8, bottom: 56 }}
          >
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
            <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-35} textAnchor="end" height={70} />
            <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} />
            <Tooltip formatter={(v: number) => fmt1(v)} contentStyle={tooltipStyle} />
            <Bar dataKey="Latência média (ms)" fill="var(--err)" radius={[4, 4, 0, 0]} />
          </BarChart>
        </ChartBox>
      </Section>

      <Section
        id="sec-alertas"
        title="4 · Alertas na janela"
        subtitle="Distribuição por tipo de alerta (equipamentos Ativos, active_since no período). O cartão de KPI mostra alertas ainda abertos."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 16 }}>
          <ChartBox h={280}>
            <PieChart margin={{ top: 8, right: 8, bottom: 56, left: 8 }}>
              <Pie
                data={alertsPie.length ? alertsPie : [{ name: "Sem dados", value: 1 }]}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="42%"
                outerRadius={72}
                label={false}
              >
                {(alertsPie.length ? alertsPie : [{ name: "—", value: 1 }]).map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Legend
                layout="horizontal"
                verticalAlign="bottom"
                align="center"
                wrapperStyle={{ fontSize: 11, lineHeight: 1.35, paddingTop: 8 }}
              />
            </PieChart>
          </ChartBox>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)" }}>Totais por tipo</h3>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Tipo</th>
                    <th className="mono">Qtd</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.alerts_by_type_30d ?? []).map((r) => (
                    <tr key={String(r.alert_type)}>
                      <td>{displayAlertType(r.alert_type)}</td>
                      <td className="mono">{fmtInt(r.count)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </Section>

      <Section
        id="sec-mikrotik"
        title="5 · Tráfego por interface (Mikrotik, última amostra)"
        subtitle="Equipamentos Ativos (categoria ou marca Mikrotik). Soma IF-MIB ifInOctets / ifOutOctets na última amostra — valores cumulativos SNMP em GiB."
      >
        {mikTrafficBar.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem snapshots de interface no período. Use Interfaces SNMP nos equipamentos ou ferramentas de walk.</p>
        ) : (
          <>
            <ChartBox h={320}>
              <BarChart data={mikTrafficBar} margin={{ left: 8, right: 8, top: 12, bottom: 52 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-30} textAnchor="end" height={72} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} />
                <Tooltip
                  formatter={(v: number, name: string) => [`${fmt1(v)} GiB`, name]}
                  contentStyle={tooltipStyle}
                  labelFormatter={(_, p) => {
                    const pl = (p as { payload?: { _rawIn?: number; _rawOut?: number } })?.payload;
                    if (!pl) return "";
                    return `${fmtBytes(pl._rawIn)} ↓ · ${fmtBytes(pl._rawOut)} ↑`;
                  }}
                />
                <Legend />
                <Bar dataKey="Entrada (GiB)" fill="#58a6ff" radius={[4, 4, 0, 0]} />
                <Bar dataKey="Saída (GiB)" fill="#3fb950" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Equipamento</th>
                    <th className="mono">Entrada</th>
                    <th className="mono">Saída</th>
                    <th>Última colheita</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.mikrotik_interface_traffic_latest ?? []).map((r) => (
                    <tr key={r.device_id}>
                      <td>{r.description}</td>
                      <td className="mono">{fmtBytes(r.if_in_octets)}</td>
                      <td className="mono">{fmtBytes(r.if_out_octets)}</td>
                      <td className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                        {r.collected_at ? new Date(r.collected_at).toLocaleString("pt-PT") : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>

      <Section
        id="sec-olt"
        title="6 · ONUs por OLT (snapshot)"
        subtitle="OLTs em operação Ativo: soma onu_total / onu_online / onu_offline nas PONs do último snapshot."
      >
        {oltOnuBar.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem snapshots OLT. Associe equipamentos OLT e execute refresh de dados OLT.</p>
        ) : (
          <>
            <div className="row" style={{ gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
              <div className="stat" style={{ minWidth: 140 }}>
                <div className="stat__k">ONUs total (todas as OLTs)</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.total)}</div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Online</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.online)}</div>
              </div>
              <div className="stat" style={{ minWidth: 120 }}>
                <div className="stat__k">Offline</div>
                <div className="stat__v">{fmtInt(oltFleetTotals.offline)}</div>
              </div>
            </div>
            <ChartBox h={300}>
              <BarChart data={oltOnuBar} margin={{ left: 8, right: 8, top: 12, bottom: 52 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-28} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip
                  contentStyle={tooltipStyle}
                  formatter={(v: number, name: string) => [`${fmtInt(v)}`, name]}
                  labelFormatter={(label, p) => {
                    const b = (p as { payload?: { brand?: string } })?.payload?.brand;
                    return b ? `${label} (${b})` : String(label);
                  }}
                />
                <Bar dataKey="Online" stackId="onu" fill="#3fb950" radius={[0, 0, 0, 0]} />
                <Bar dataKey="Offline" stackId="onu" fill="#f85149" radius={[4, 4, 0, 0]} />
                <Legend />
              </BarChart>
            </ChartBox>
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>OLT</th>
                    <th>Marca</th>
                    <th className="mono">Total</th>
                    <th className="mono">Online</th>
                    <th className="mono">Offline</th>
                    <th>Snapshot</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.olt_onu_by_device ?? []).map((r) => (
                    <tr key={r.device_id}>
                      <td>{r.description}</td>
                      <td>{r.brand ?? "—"}</td>
                      <td className="mono">{fmtInt(r.onu_count)}</td>
                      <td className="mono">{fmtInt(r.onu_online)}</td>
                      <td className="mono">{fmtInt(r.onu_offline)}</td>
                      <td className="mono" style={{ fontSize: 10, color: "var(--muted)" }}>
                        {r.snapshot_at ? new Date(r.snapshot_at).toLocaleString("pt-PT") : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>

      <Section
        id="sec-olt-capacity"
        title="7 · Capacidade OLT por PON"
        subtitle="Percentual de ocupação por PON (base 128 ONUs/PON) e tendência total de ONUs nos últimos 7 dias."
      >
        {cap.isError && <div className="msg msg--err">{(cap.error as Error).message}</div>}
        {cap.data && (
          <>
            <ChartBox h={280}>
              <BarChart
                data={(cap.data.pon_rows ?? [])
                  .slice(0, 20)
                  .map((p) => ({ name: `${trunc(p.olt, 12)}:${p.pon_id}`, "% uso": Number(p.usage_percent ?? 0) }))}
                margin={{ left: 8, right: 8, top: 12, bottom: 52 }}
              >
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" tick={{ fill: "var(--muted)", fontSize: 9 }} interval={0} angle={-28} textAnchor="end" height={70} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} />
                <Tooltip contentStyle={tooltipStyle} />
                <Bar dataKey="% uso" fill="#d29922" />
              </BarChart>
            </ChartBox>
            <div className="table-wrap" style={{ marginTop: 10 }}>
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>OLT</th>
                    <th>PON</th>
                    <th className="mono">ONU total</th>
                    <th className="mono">% uso</th>
                    <th>Alerta</th>
                  </tr>
                </thead>
                <tbody>
                  {(cap.data.pon_rows ?? []).slice(0, 30).map((p, i) => (
                    <tr key={`${p.olt_id}-${p.pon_id}-${i}`}>
                      <td>{p.olt}</td>
                      <td className="mono">{p.pon_id}</td>
                      <td className="mono">{fmtInt(p.onu_total)}</td>
                      <td className="mono">{Number(p.usage_percent ?? 0).toFixed(1)}%</td>
                      <td>{p.near_saturation ? <span className="badge badge--err">próx. saturação</span> : <span className="badge badge--ok">ok</span>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </Section>

    </div>
  );
}

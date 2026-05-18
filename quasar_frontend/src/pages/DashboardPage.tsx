import { useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { DashboardPageLoader } from "../components/DashboardPageLoader";
import { InfoHint } from "../components/InfoHint";
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ComposedChart,
  Legend,
  Line,
  LineChart,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { apiFetch } from "../lib/api";

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

type PingDay = { day?: string; samples?: number; ok_percent?: number; avg_latency_ms?: number | null };
type TelDay = { day?: string; samples?: number };
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
  ping_window?: { samples?: number; ok_samples?: number; ok_percent?: number; avg_latency_ms?: number | null };
  ping_by_day?: PingDay[];
  ping_ranking_worst_latency?: LatRank[];
  ping_ranking_best_latency?: LatRank[];
  telemetry_window?: { samples?: number };
  telemetry_by_day?: TelDay[];
  alerts_by_type_30d?: NamedCount[];
  alerts_open?: number;
  mikrotik_interface_traffic_latest?: MikTraffic[];
  olt_onu_by_device?: OltOnu[];
};
type DataGapSummary = {
  without_locality?: number;
  without_ip?: number;
  without_snmp_community?: number;
  without_coordinates?: number;
  without_telemetry?: number;
};
type DataGapDevice = { id: string; description: string; category: string; gaps: string[] };
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
      <h2 style={{ marginBottom: subtitle ? 4 : 8, color: "var(--text)", textTransform: "none", letterSpacing: 0 }}>{title}</h2>
      {subtitle && (
        <p style={{ margin: "0 0 12px", fontSize: 12, color: "var(--muted)", maxWidth: 900 }}>
          {subtitle}
        </p>
      )}
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
  const [days, setDays] = useState(30);
  const [catViz, setCatViz] = useState<"pie" | "bar">("pie");
  const [pageIn, setPageIn] = useState(false);

  const dash = useQuery({
    queryKey: ["dashboard-analytics", days],
    queryFn: () => apiFetch<DashboardAnalytics>(`/api/v1/dashboard/analytics?days=${days}`),
  });

  const topProbe = useQuery({
    queryKey: ["top-latency"],
    queryFn: () => apiFetch<{ top: TopRow[] }>("/api/v1/overview/top-latency?limit=8"),
  });
  const gaps = useQuery({
    queryKey: ["dashboard-data-gaps"],
    queryFn: () => apiFetch<{ summary: DataGapSummary; devices: DataGapDevice[] }>("/api/v1/dashboard/data-gaps"),
  });
  const cap = useQuery({
    queryKey: ["dashboard-olt-capacity"],
    queryFn: () => apiFetch<OltCapacity>("/api/v1/dashboard/olt-capacity"),
  });

  const totals = dash.data?.totals;

  const pingDays = useMemo(() => {
    const rows = dash.data?.ping_by_day ?? [];
    return rows.map((r) => ({
      day: String(r.day ?? "").slice(5),
      samples: num(r.samples),
      ok_percent: r.ok_percent != null ? num(r.ok_percent) : null,
      avg_latency_ms: r.avg_latency_ms != null ? num(r.avg_latency_ms) : null,
    }));
  }, [dash.data?.ping_by_day]);

  const telDays = useMemo(() => {
    return (dash.data?.telemetry_by_day ?? []).map((r) => ({
      day: String(r.day ?? "").slice(5),
      samples: num(r.samples),
    }));
  }, [dash.data?.telemetry_by_day]);

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
      name: String(r.alert_type ?? "—"),
      value: num(r.count),
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

  if (dash.isLoading && !dash.data) return <DashboardPageLoader />;
  if (dash.isError) return <div className="msg msg--err">{(dash.error as Error).message}</div>;

  const pw = dash.data?.ping_window;

  return (
    <div className={`dashboard-page${pageIn ? " dashboard-page--in" : ""}`}>
      <h1>
        Dashboard analítico
        <InfoHint label="Sobre o dashboard analítico">
          <p>
            Visão agregada dos últimos <strong>{dash.data?.days ?? days}</strong> dias (dados materializados: ping, telemetria, alertas,
            snapshots). Ajuste o período para comparar tendências. Secções independentes — pode fazer scroll por tema.
          </p>
        </InfoHint>
      </h1>

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
        </span>
      </div>

      {/* KPIs */}
      <div className="grid-cards" style={{ marginTop: 4 }}>
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
          <div className="stat__k">Amostras ping (janela)</div>
          <div className="stat__v">{fmtInt(pw?.samples)}</div>
          <div className="mono" style={{ fontSize: 11, color: "var(--muted)" }}>
            OK médio: {fmt1(pw?.ok_percent)}% · latência média: {pw?.avg_latency_ms != null ? `${fmt1(pw.avg_latency_ms)} ms` : "—"}
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
            <ChartBox h={280}>
              <PieChart>
                <Pie data={catPie} dataKey="value" nameKey="name" cx="50%" cy="50%" outerRadius={100} label={({ name, percent }) => `${name} (${(percent * 100).toFixed(0)}%)`}>
                  {catPie.map((_, i) => (
                    <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                  ))}
                </Pie>
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Legend />
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
        id="sec-ping"
        title="3 · Ping na janela temporal"
        subtitle="À esquerda: volume de amostras por dia. À direita: qualidade — taxa média de sucesso (%) e latência média em ms (só pings OK com latência); escalas independentes."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))", gap: 16 }}>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Volume de amostras / dia</h3>
            <ChartBox h={260}>
              <BarChart data={pingDays} margin={{ left: 8, right: 8, top: 8, bottom: 8 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="day" tick={{ fill: "var(--muted)", fontSize: 10 }} />
                <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
                <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
                <Bar dataKey="samples" name="Amostras" fill="#58a6ff" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ChartBox>
          </div>
          <div>
            <h3 style={{ fontSize: 12, color: "var(--muted)", marginBottom: 6 }}>Qualidade (média diária)</h3>
            <ChartBox h={260}>
              <ComposedChart data={pingDays} margin={{ left: 8, right: 12, top: 8, bottom: 8 }}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="day" tick={{ fill: "var(--muted)", fontSize: 10 }} />
                <YAxis yAxisId="pct" domain={[0, 100]} tick={{ fill: "var(--muted)", fontSize: 10 }} width={36} />
                <YAxis yAxisId="ms" orientation="right" tick={{ fill: "var(--muted)", fontSize: 10 }} width={44} />
                <Tooltip contentStyle={tooltipStyle} />
                <Legend />
                <Line yAxisId="pct" type="monotone" dataKey="ok_percent" name="OK %" stroke="#3fb950" dot strokeWidth={2} connectNulls />
                <Line yAxisId="ms" type="monotone" dataKey="avg_latency_ms" name="Latência (ms)" stroke="#ffa657" dot strokeWidth={2} connectNulls />
              </ComposedChart>
            </ChartBox>
          </div>
        </div>
      </Section>

      <Section
        id="sec-rankings"
        title="4 · Rankings de latência (média na janela)"
        subtitle="Equipamentos com pelo menos 3 amostras válidas: piores médias (investigar congestão) e melhores médias. À direita, última latência na cache de sondas (instantâneo)."
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
                    <th className="mono">N</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.ping_ranking_worst_latency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                      <td className="mono">{fmtInt(r.samples)}</td>
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
                    <th className="mono">N</th>
                  </tr>
                </thead>
                <tbody>
                  {(dash.data?.ping_ranking_best_latency ?? []).map((r, i) => (
                    <tr key={r.device_id ?? i}>
                      <td>{i + 1}</td>
                      <td>{trunc(String(r.description ?? ""), 28)}</td>
                      <td className="mono">{fmt1(r.avg_latency_ms)}</td>
                      <td className="mono">{fmtInt(r.samples)}</td>
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
        id="sec-telemetria"
        title="5 · Telemetria SNMP (volume)"
        subtitle="Contagem de amostras gravadas por dia — reflexo do ciclo configurado e do número de equipamentos com telemetria activa."
      >
        <ChartBox h={280}>
          <LineChart data={telDays} margin={{ left: 8, right: 16, top: 12, bottom: 8 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
            <XAxis dataKey="day" tick={{ fill: "var(--muted)", fontSize: 10 }} />
            <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} allowDecimals={false} />
            <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
            <Legend />
            <Line type="monotone" dataKey="samples" name="Amostras / dia" stroke="#79c0ff" strokeWidth={2} dot />
          </LineChart>
        </ChartBox>
      </Section>

      <Section
        id="sec-alertas"
        title="6 · Alertas na janela"
        subtitle="Distribuição por tipo de alerta (instâncias com active_since dentro do período). O cartão de KPI mostra alertas ainda abertos (sem closed_at)."
      >
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 16 }}>
          <ChartBox h={260}>
            <PieChart>
              <Pie data={alertsPie.length ? alertsPie : [{ name: "Sem dados", value: 1 }]} dataKey="value" nameKey="name" cx="50%" cy="50%" outerRadius={90} label>
                {(alertsPie.length ? alertsPie : [{ name: "—", value: 1 }]).map((_, i) => (
                  <Cell key={i} fill={CHART_COLORS[i % CHART_COLORS.length]} />
                ))}
              </Pie>
              <Tooltip formatter={(v: number) => fmtInt(v)} contentStyle={tooltipStyle} />
              <Legend />
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
                      <td className="mono">{String(r.alert_type)}</td>
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
        title="7 · Tráfego por interface (Mikrotik, última amostra)"
        subtitle="Soma de contadores IF-MIB ifInOctets (.10.*) e ifOutOctets (.16.*) no último snapshot de interfaces por equipamento (categoria ou marca contém «mikrotik»). Valores cumulativos SNMP — barras em GiB para leitura humana."
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
        title="8 · ONUs por OLT (snapshot)"
        subtitle="Soma de onu_total / onu_online / onu_offline nas linhas «pons» (SNMP MIB OLT ou derivado das interfaces IF-MIB após «Actualizar interfaces» na OLT)."
      >
        {oltOnuBar.length === 0 ? (
          <p style={{ color: "var(--muted)", fontSize: 13 }}>Sem snapshots OLT. Associe equipamentos OLT e execute refresh de dados OLT.</p>
        ) : (
          <>
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
        title="9 · Capacidade OLT por PON"
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

      <Section
        id="sec-data-gaps"
        title="10 · Painel de dados faltantes"
        subtitle="Dispositivos com cadastro incompleto para operação/monitoramento."
      >
        {gaps.isError && <div className="msg msg--err">{(gaps.error as Error).message}</div>}
        {gaps.data && (
          <>
            <div className="grid-cards" style={{ marginBottom: 10 }}>
              <div className="stat"><div className="stat__k">Sem localidade</div><div className="stat__v">{fmtInt(gaps.data.summary.without_locality)}</div></div>
              <div className="stat"><div className="stat__k">Sem IP</div><div className="stat__v">{fmtInt(gaps.data.summary.without_ip)}</div></div>
              <div className="stat"><div className="stat__k">Sem community</div><div className="stat__v">{fmtInt(gaps.data.summary.without_snmp_community)}</div></div>
              <div className="stat"><div className="stat__k">Sem coordenadas</div><div className="stat__v">{fmtInt(gaps.data.summary.without_coordinates)}</div></div>
              <div className="stat"><div className="stat__k">Sem telemetria</div><div className="stat__v">{fmtInt(gaps.data.summary.without_telemetry)}</div></div>
            </div>
            <div className="table-wrap">
              <table style={{ fontSize: 11 }}>
                <thead>
                  <tr>
                    <th>Equipamento</th>
                    <th>Categoria</th>
                    <th>Pendências</th>
                  </tr>
                </thead>
                <tbody>
                  {(gaps.data.devices ?? []).slice(0, 80).map((d) => (
                    <tr key={d.id}>
                      <td>{d.description}</td>
                      <td>{d.category}</td>
                      <td className="mono">{(d.gaps ?? []).join(", ")}</td>
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

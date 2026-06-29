import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import { AlertTriangle, Eye, Filter, Loader2, RefreshCw, Search } from "lucide-react";
import {
  CartesianGrid,
  Cell,
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
import { PageCountPill } from "../components/PageCountPill";
import { ConfirmModal } from "../components/ConfirmModal";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import {
  BNG_SESSION_DISPLAY_LIMITS,
  formatBngDateTime,
  formatBngDuration,
  formatBngIpv6Display,
  formatBngIpType,
  formatBngKbitRate,
  formatBngSessionStatus,
  formatOverviewField,
  sessionDisplayDnLimit,
  sessionDisplayOnline,
  sessionDisplayUpLimit,
  OVERVIEW_FIELD_LABELS,
  STATS_SERIES,
  type BngSessionDisplayLimit,
  type OverviewFieldKey,
  type StatsSeriesKey,
} from "../lib/bngDisplay";
import { useAppToast } from "../lib/appToast";
import {
  BNG_SESSION_SEARCH_FIELDS,
  countActiveBngSessionFilters,
  EMPTY_BNG_SESSION_FILTERS,
  filterBngSessions,
  type BngSessionAdvancedFilters,
  type BngSessionSearchField,
} from "../lib/bngSessionFilters";
import { toastErr, toastOk } from "../lib/operationToast";
import { APP_ROUTES } from "../app/routes";

type BngDevice = {
  id: string;
  description?: string;
  ip?: string;
};

type BngOverview = {
  device?: BngDevice;
  telemetry_collected_at?: string;
  fields?: Record<string, string | number>;
  latest_stats?: {
    collected_at?: string;
    total_online?: number | null;
    pppoe_online?: number | null;
    ipv4_online?: number | null;
    ipv6_online?: number | null;
    dual_stack_online?: number | null;
  };
};

type StatsSample = {
  collected_at: string;
  total_online?: number | null;
  pppoe_online?: number | null;
  ipv4_online?: number | null;
  ipv6_online?: number | null;
  dual_stack_online?: number | null;
};

type PppoeSession = {
  index?: string;
  login?: string;
  ipv4?: string;
  mac?: string;
  ipv6?: string;
  ipv6_pd?: string;
  ip_type?: string;
  ip_type_raw?: string;
  online_time?: string;
  online_time_sec?: string;
  status?: string;
  auth_state?: string;
  author_state?: string;
  acct_state?: string;
  port_type?: string;
  vlan?: string;
  interface?: string;
  domain?: string;
  up_flow_bytes?: string;
  dn_flow_bytes?: string;
  car_up_cir_kbps?: string;
  car_dn_cir_kbps?: string;
  car_up_cir_display?: string;
  car_dn_cir_display?: string;
  up_flow_display?: string;
  dn_flow_display?: string;
  qos_profile?: string;
};

type SessionBreakdown = {
  session_count: number;
  by_vlan: { key: string; label: string; count: number }[];
  by_up_limit: { kbps: number; label: string; display: string; count: number }[];
  by_dn_limit: { kbps: number; label: string; display: string; count: number }[];
  online_time: {
    avg_seconds?: number;
    avg_display?: string;
    max_seconds?: number;
    max_display?: string;
    with_data?: number;
    buckets: { key: string; label: string; count: number }[];
  };
  traffic?: {
    up_flow64_total?: number;
    dn_flow64_total?: number;
    up_flow_display?: string;
    dn_flow_display?: string;
    sessions_with_up?: number;
    sessions_with_dn?: number;
  };
};

type BngIPPoolRow = {
  index?: string;
  name?: string;
  total_ips?: number;
  used_ips?: number;
  idle_ips?: number;
  used_percent?: number;
  vrf?: string;
  gateway?: string;
};

type BngIPv6PoolRow = {
  index?: string;
  name?: string;
  address_total?: number;
  address_used?: number;
  address_free?: number;
  address_used_percent?: number;
  pd_prefix_total?: number;
  pd_prefix_used?: number;
  pd_prefix_free?: number;
  pd_prefix_used_percent?: number;
};

type BngRadiusRow = {
  key?: string;
  type?: string;
  ip?: string;
  port?: string;
  responses?: string;
  vrf?: string;
};

type BngAAAScalars = {
  historic_max_online?: string;
  max_pppoe_online?: string;
  total_connect?: string;
  total_success?: string;
  total_authen_fail?: string;
  total_ppp_fail?: string;
  total_lcp_fail?: string;
  total_ip_alloc_fail?: string;
  ipv4_flow_up_bytes?: string;
  ipv4_flow_dn_bytes?: string;
  ipv6_flow_up_bytes?: string;
  ipv6_flow_dn_bytes?: string;
  wired_online?: string;
  vlan_online?: string;
};

type BngCGN = {
  current_sessions?: string;
  license_total_m?: string;
  license_used_m?: string;
  license_free_m?: string;
  bit_throughput_up?: string;
  bit_throughput_down?: string;
  dslite_tunnels?: string;
};

type BngInfrastructure = {
  collected_at?: string;
  aaa_scalars?: BngAAAScalars;
  power_consumption?: string;
  ipv4_pools?: BngIPPoolRow[];
  ipv6_pools?: BngIPv6PoolRow[];
  radius_servers?: BngRadiusRow[];
  cgn?: BngCGN;
};

type SessionReportResponse = {
  captured_at?: string;
  note?: string;
  session_count?: number;
  report?: SessionBreakdown;
  infrastructure?: BngInfrastructure;
  infrastructure_captured_at?: string;
  infrastructure_note?: string;
};

type AuthAttemptLog = {
  kind: "success" | "failure";
  time?: string;
  login?: string;
  mac?: string;
  port?: string;
  reason?: string;
  detail?: string;
  message?: string;
};

type TrafficRateSnapshot = {
  up_bps?: number;
  dn_bps?: number;
  up_display?: string;
  dn_display?: string;
  interval_ms?: number;
  sampled_at?: string;
};

type BngTab = "overview" | "relatorio" | "auth" | "sessions";

type BngCollectProgress = {
  status: string;
  phase?: string;
  logins_loaded?: number;
  sessions_enriched?: number;
  sessions_total?: number;
  session_count?: number;
  message?: string;
  error?: string;
  done?: boolean;
};

const OVERVIEW_KEYS: OverviewFieldKey[] = ["sys_name", "sys_uptime", "cpu_usage", "memory_usage", "temperature"];

const BNG_CHART_COLORS = ["#58a6ff", "#3fb950", "#d29922", "#f85149", "#a371f7", "#79c0ff", "#ff7b72", "#56d364", "#ffa657"];

function sleep(ms: number) {
  return new Promise((r) => setTimeout(r, ms));
}

function progressLoginLabel(n?: number) {
  if (!n || n <= 0) return "0";
  if (n < 50) return String(n);
  if (n < 500) return String(Math.floor(n / 50) * 50);
  return String(Math.floor(n / 500) * 500);
}

function renderAuthLogMessage(r: AuthAttemptLog, onLoginClick?: (login: string) => void) {
  const login = r.login?.trim();
  const line = r.message?.trim() || `${r.time || "—"} · ${login || "—"} · ${r.mac || "—"}`;
  if (!login || !onLoginClick) return line;
  const needle = `[${login}/]`;
  const idx = line.indexOf(needle);
  if (idx < 0) return line;
  return (
    <>
      {line.slice(0, idx)}
      <button
        type="button"
        onClick={() => onLoginClick(login)}
        style={{
          background: "none",
          border: "none",
          padding: 0,
          margin: 0,
          font: "inherit",
          color: "var(--accent, #58a6ff)",
          cursor: "pointer",
          textDecoration: "underline",
        }}
      >
        {needle}
      </button>
      {line.slice(idx + needle.length)}
    </>
  );
}

function BngAuthRecordsPanel({
  data,
  loading,
  refreshing,
  onLoginClick,
}: {
  data?: { records?: AuthAttemptLog[]; count?: number; note?: string; fetched_at?: string };
  loading: boolean;
  refreshing?: boolean;
  onLoginClick?: (login: string) => void;
}) {
  const [filter, setFilter] = useState("");
  const rows = useMemo(() => {
    const list = data?.records ?? [];
    const q = filter.trim().toLowerCase();
    if (!q) return list;
    return list.filter(
      (r) =>
        r.login?.toLowerCase().includes(q) ||
        r.mac?.toLowerCase().includes(q) ||
        r.message?.toLowerCase().includes(q) ||
        r.reason?.toLowerCase().includes(q) ||
        r.detail?.toLowerCase().includes(q),
    );
  }, [data?.records, filter]);

  return (
    <div className="card" style={{ padding: 14 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, marginBottom: 12, flexWrap: "wrap" }}>
        <div>
          <h3 style={{ margin: "0 0 4px", fontSize: 15 }}>Log de autenticação AAA</h3>
          <p style={{ margin: 0, fontSize: 12, color: "var(--muted)" }}>
            Tentativas recentes de autenticação no BNG — falhas AAA + logins OK detectados em tempo real (atualização a cada 2 s).
            {refreshing && (
              <span style={{ marginLeft: 8 }}>
                <Loader2 size={12} className="map-refresh-spin" aria-hidden /> A actualizar…
              </span>
            )}
          </p>
        </div>
        <input
          className="input"
          placeholder="Filtrar login ou motivo…"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          style={{ maxWidth: 260 }}
        />
      </div>
      {loading ? (
        <p style={{ fontSize: 13, color: "var(--muted)", display: "flex", alignItems: "center", gap: 8 }}>
          <Loader2 size={16} className="map-refresh-spin" aria-hidden /> A consultar registos AAA via SNMP…
        </p>
      ) : rows.length === 0 ? (
        <p style={{ fontSize: 13, color: "var(--muted)" }}>{data?.note || "Sem registos AAA disponíveis."}</p>
      ) : (
        <div style={{ display: "flex", flexDirection: "column", gap: 6, maxHeight: 520, overflow: "auto" }}>
          {rows.map((r, i) => {
            const ok = r.kind === "success";
            return (
              <div
                key={`${r.time}-${r.login}-${i}`}
                style={{
                  padding: "8px 10px",
                  borderRadius: 6,
                  borderLeft: `3px solid ${ok ? "#3fb950" : "#f85149"}`,
                  background: ok ? "rgba(63,185,80,0.06)" : "rgba(248,81,73,0.06)",
                  fontSize: 12,
                  fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
                  lineHeight: 1.45,
                  wordBreak: "break-word",
                }}
              >
                {renderAuthLogMessage(r, onLoginClick)}
              </div>
            );
          })}
        </div>
      )}
      {!loading && (data?.count ?? 0) > 0 && (
        <p style={{ fontSize: 11, color: "var(--muted)", margin: "10px 0 0" }}>
          {rows.length.toLocaleString("pt-PT")} registo(s) exibido(s)
          {filter.trim() ? ` (filtrado de ${data!.count})` : ""}.
        </p>
      )}
    </div>
  );
}

function VlanPieSection({ rows }: { rows: { label: string; count: number }[] }) {
  const [expanded, setExpanded] = useState(false);
  const pieData = useMemo(
    () => rows.map((r) => ({ name: r.label, value: r.count })),
    [rows],
  );
  if (rows.length === 0) return null;
  return (
    <div className="card" style={{ padding: 14 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, marginBottom: 10 }}>
        <h3 style={{ margin: 0, fontSize: 15 }}>Logins por VLAN</h3>
        <button type="button" className="btn btn--sm" onClick={() => setExpanded((v) => !v)}>
          {expanded ? "Ocultar tabela" : "Ver tabela"}
        </button>
      </div>
      <ResponsiveContainer width="100%" height={260}>
        <PieChart margin={{ top: 8, right: 8, bottom: 48, left: 8 }}>
          <Pie data={pieData} dataKey="value" nameKey="name" cx="50%" cy="42%" outerRadius={78} label={false}>
            {pieData.map((_, i) => (
              <Cell key={i} fill={BNG_CHART_COLORS[i % BNG_CHART_COLORS.length]} />
            ))}
          </Pie>
          <Tooltip formatter={(v: number) => [v.toLocaleString("pt-PT"), "Logins"]} />
          <Legend layout="horizontal" verticalAlign="bottom" align="center" wrapperStyle={{ fontSize: 11, lineHeight: 1.35, paddingTop: 8 }} />
        </PieChart>
      </ResponsiveContainer>
      {expanded && (
        <div className="table-wrap" style={{ marginTop: 12 }}>
          <table>
            <thead>
              <tr>
                <th>VLAN</th>
                <th style={{ width: 100, textAlign: "right" }}>Logins</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.label}>
                  <td>{r.label}</td>
                  <td style={{ textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{r.count.toLocaleString("pt-PT")}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function SessionDetailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <div
      className="card"
      style={{
        padding: 12,
        margin: 0,
        background: "var(--surface-2, rgba(255,255,255,0.03))",
      }}
    >
      <h4 style={{ margin: "0 0 10px", fontSize: 13, fontWeight: 600 }}>{title}</h4>
      {children}
    </div>
  );
}

function DetailField({ label, value }: { label: string; value?: string }) {
  return (
    <div>
      <div style={{ fontSize: 11, color: "var(--muted)" }}>{label}</div>
      <div className="mono" style={{ fontSize: 12, wordBreak: "break-word" }}>
        {value || "—"}
      </div>
    </div>
  );
}

function BngInfrastructureReport({ infra, capturedAt, note }: { infra?: BngInfrastructure; capturedAt?: string; note?: string }) {
  if (!infra) {
    return note ? (
      <div className="msg msg--warn" style={{ marginBottom: 16 }}>
        {note}
      </div>
    ) : null;
  }
  const aaa = infra.aaa_scalars;
  const poolTotals = (infra.ipv4_pools ?? []).reduce(
    (acc, p) => ({
      total: acc.total + (p.total_ips ?? 0),
      used: acc.used + (p.used_ips ?? 0),
      idle: acc.idle + (p.idle_ips ?? 0),
    }),
    { total: 0, used: 0, idle: 0 },
  );

  return (
    <>
      <h3 style={{ fontSize: 14, margin: "0 0 10px", color: "var(--muted)", fontWeight: 600 }}>
        Infraestrutura BNG
        {capturedAt ? ` · ${formatBngDateTime(capturedAt)}` : ""}
      </h3>

      <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
        {[
          { label: "Pico histórico online", value: aaa?.historic_max_online },
          { label: "Tentativas PPPoE", value: aaa?.total_connect },
          { label: "Sucesso PPPoE", value: aaa?.total_success },
          { label: "Falhas autenticação", value: aaa?.total_authen_fail },
          { label: "Falhas PPPoE", value: aaa?.total_ppp_fail },
          { label: "Energia acumulada", value: infra.power_consumption },
        ].map((c) => (
          <div key={c.label} className="card" style={{ padding: "10px 14px", minWidth: 130 }}>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>{c.label}</div>
            <strong style={{ fontSize: 16 }}>{c.value ?? "—"}</strong>
          </div>
        ))}
      </div>

      {poolTotals.total > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools IPv4 (totais)</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 12 }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>IPs totais</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.total.toLocaleString("pt-PT")}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Em uso</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.used.toLocaleString("pt-PT")}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Ociosos</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{poolTotals.idle.toLocaleString("pt-PT")}</div>
            </div>
          </div>
          {(infra.ipv4_pools ?? []).length > 0 && (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Pool</th>
                    <th>VRF</th>
                    <th>Total</th>
                    <th>Usados</th>
                    <th>Ociosos</th>
                    <th>% uso</th>
                  </tr>
                </thead>
                <tbody>
                  {infra.ipv4_pools!.map((p) => (
                    <tr key={`${p.index}-${p.name}`}>
                      <td className="mono">{p.name}</td>
                      <td className="mono">{p.vrf || "—"}</td>
                      <td>{p.total_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.used_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.idle_ips?.toLocaleString("pt-PT") ?? "—"}</td>
                      <td>{p.used_percent != null ? `${p.used_percent}%` : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {(infra.ipv6_pools ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools IPv6</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Pool</th>
                  <th>Endereços %</th>
                  <th>PD %</th>
                  <th>Usados / total</th>
                </tr>
              </thead>
              <tbody>
                {infra.ipv6_pools!.map((p) => (
                  <tr key={`${p.index}-${p.name}`}>
                    <td className="mono">{p.name}</td>
                    <td>{p.address_used_percent != null ? `${p.address_used_percent}%` : "—"}</td>
                    <td>{p.pd_prefix_used_percent != null ? `${p.pd_prefix_used_percent}%` : "—"}</td>
                    <td>
                      {p.address_used ?? "—"} / {p.address_total ?? "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {(infra.radius_servers ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Servidores RADIUS</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Tipo</th>
                  <th>IP</th>
                  <th>Porta</th>
                  <th>Respostas</th>
                  <th>VRF</th>
                </tr>
              </thead>
              <tbody>
                {infra.radius_servers!.map((r) => (
                  <tr key={r.key ?? `${r.ip}-${r.port}`}>
                    <td>{r.type || "—"}</td>
                    <td className="mono">{r.ip || "—"}</td>
                    <td className="mono">{r.port || "—"}</td>
                    <td>{r.responses ?? "—"}</td>
                    <td className="mono">{r.vrf || "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {infra.cgn && Object.values(infra.cgn).some(Boolean) && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>CGN / NAT</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            {[
              { label: "Sessões actuais", value: infra.cgn.current_sessions },
              { label: "Licença total (M)", value: infra.cgn.license_total_m },
              { label: "Licença usada (M)", value: infra.cgn.license_used_m },
              { label: "Licença livre (M)", value: infra.cgn.license_free_m },
              { label: "Throughput up (bits)", value: infra.cgn.bit_throughput_up },
              { label: "Throughput down (bits)", value: infra.cgn.bit_throughput_down },
            ].map((c) => (
              <div key={c.label}>
                <div style={{ fontSize: 11, color: "var(--muted)" }}>{c.label}</div>
                <div style={{ fontSize: 15, fontWeight: 600 }}>{c.value ?? "—"}</div>
              </div>
            ))}
          </div>
        </div>
      )}
    </>
  );
}

function BreakdownTable({
  title,
  rows,
  valueHeader,
}: {
  title: string;
  rows: { label: string; count: number }[];
  valueHeader: string;
}) {
  if (rows.length === 0) return null;
  return (
    <div className="card" style={{ padding: 14 }}>
      <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>{title}</h3>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>{valueHeader}</th>
              <th style={{ width: 100, textAlign: "right" }}>Logins</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.label}>
                <td>{r.label}</td>
                <td style={{ textAlign: "right", fontVariantNumeric: "tabular-nums" }}>{r.count.toLocaleString("pt-PT")}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function BngSessionReportPanel({ data, loading }: { data?: SessionReportResponse; loading: boolean }) {
  if (loading) return <p style={{ fontSize: 13, color: "var(--muted)" }}>A carregar relatório de sessões…</p>;
  const rep = data?.report;
  if (!rep || rep.session_count === 0) {
    return (
      <div className="msg msg--warn" style={{ marginBottom: 16 }}>
        {data?.note || "Execute a consulta completa SNMP na aba Sessões PPPoE para gerar o relatório por VLAN, limites e tempo online."}
      </div>
    );
  }
  return (
    <>
      <div
        style={{
          marginBottom: 12,
          padding: "8px 12px",
          borderRadius: 8,
          background: "var(--surface-2, rgba(255,255,255,0.04))",
          border: "1px solid var(--border)",
          display: "inline-flex",
          flexDirection: "column",
          gap: 2,
        }}
      >
        <span style={{ fontSize: 11, color: "var(--muted)" }}>Snapshot de sessões</span>
        <span style={{ fontSize: 14, fontWeight: 600 }}>
          {rep.session_count.toLocaleString("pt-PT")} logins
          {data?.captured_at ? ` · ${formatBngDateTime(data.captured_at)}` : ""}
        </span>
      </div>

      <div className="card" style={{ padding: 14, marginBottom: 16 }}>
        <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Tempo online</h3>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 12 }}>
          <div>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Média</div>
            <div style={{ fontSize: 18, fontWeight: 600 }}>{rep.online_time.avg_display || "—"}</div>
          </div>
          <div>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Máximo</div>
            <div style={{ fontSize: 18, fontWeight: 600 }}>{rep.online_time.max_display || "—"}</div>
          </div>
          <div>
            <div style={{ fontSize: 11, color: "var(--muted)" }}>Com dado SNMP</div>
            <div style={{ fontSize: 18, fontWeight: 600 }}>{rep.online_time.with_data?.toLocaleString("pt-PT") ?? "—"}</div>
          </div>
        </div>
        {rep.online_time.buckets.length > 0 && (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Faixa</th>
                  <th style={{ width: 100, textAlign: "right" }}>Logins</th>
                </tr>
              </thead>
              <tbody>
                {rep.online_time.buckets.map((b) => (
                  <tr key={b.key}>
                    <td>{b.label}</td>
                    <td style={{ textAlign: "right" }}>{b.count.toLocaleString("pt-PT")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {rep.traffic && (rep.traffic.up_flow_display || rep.traffic.dn_flow_display) && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Tráfego total PPPoE (Flow64)</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 24 }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Upstream agregado</div>
              <div style={{ fontSize: 20, fontWeight: 600 }}>{rep.traffic.up_flow_display || "—"}</div>
              {rep.traffic.sessions_with_up != null && (
                <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 2 }}>
                  {rep.traffic.sessions_with_up.toLocaleString("pt-PT")} sessões com dado
                </div>
              )}
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Downstream agregado</div>
              <div style={{ fontSize: 20, fontWeight: 600 }}>{rep.traffic.dn_flow_display || "—"}</div>
              {rep.traffic.sessions_with_dn != null && (
                <div style={{ fontSize: 11, color: "var(--muted)", marginTop: 2 }}>
                  {rep.traffic.sessions_with_dn.toLocaleString("pt-PT")} sessões com dado
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      <BngInfrastructureReport
        infra={data?.infrastructure}
        capturedAt={data?.infrastructure_captured_at}
        note={data?.infrastructure_note}
      />
    </>
  );
}

function BngStatsMiniChart({
  samples,
  seriesKey,
  color,
  label,
  height = 160,
}: {
  samples: StatsSample[];
  seriesKey: StatsSeriesKey;
  color: string;
  label: string;
  height?: number;
}) {
  const { data, lastValue, hasEnough } = useMemo(() => {
    const sorted = [...samples].sort(
      (a, b) => new Date(a.collected_at).getTime() - new Date(b.collected_at).getTime(),
    );
    const rows = sorted.map((p) => {
      const raw = p[seriesKey];
      return {
        ts: p.collected_at,
        label: new Date(p.collected_at).toLocaleString("pt-BR", {
          day: "2-digit",
          month: "2-digit",
          hour: "2-digit",
          minute: "2-digit",
        }),
        value: raw == null ? null : Number(raw),
      };
    });
    const numeric = rows.map((r) => r.value).filter((v): v is number => v != null && Number.isFinite(v));
    const last = numeric.length > 0 ? numeric[numeric.length - 1] : null;
    return { data: rows, lastValue: last, hasEnough: numeric.length >= 2 };
  }, [samples, seriesKey]);

  const yDomain = useMemo(() => {
    const numeric = data.map((d) => d.value).filter((v): v is number => v != null && Number.isFinite(v));
    if (numeric.length === 0) return [0, 1] as [number, number];
    const min = Math.min(...numeric);
    const max = Math.max(...numeric);
    const span = Math.max(1, max - min);
    const pad = Math.max(1, Math.round(span * 0.1));
    return [min - pad, max + pad] as [number, number];
  }, [data]);

  if (!hasEnough) {
    return (
      <div className="card" style={{ padding: 12, margin: 0 }}>
        <div style={{ fontSize: 12, fontWeight: 600, marginBottom: 6 }}>{label}</div>
        <p style={{ fontSize: 11, color: "var(--muted)", margin: 0 }}>
          Aguardando histórico (mín. 2 coletas com valor para esta métrica).
        </p>
      </div>
    );
  }

  return (
    <div className="card" style={{ padding: 12, margin: 0 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", marginBottom: 6 }}>
        <span style={{ fontSize: 12, fontWeight: 600 }}>{label}</span>
        <span className="mono" style={{ fontSize: 18, color }}>
          {lastValue ?? "—"}
        </span>
      </div>
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={data} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
          <XAxis dataKey="label" tick={{ fontSize: 9 }} interval="preserveStartEnd" minTickGap={24} />
          <YAxis tick={{ fontSize: 10 }} width={44} allowDecimals={false} domain={yDomain} />
          <Tooltip
            labelFormatter={(_, payload) => {
              const ts = payload?.[0]?.payload?.ts;
              return ts ? new Date(String(ts)).toLocaleString("pt-BR") : "";
            }}
            formatter={(value: number | null) => [value == null ? "—" : value, label]}
          />
          <Line type="monotone" dataKey="value" stroke={color} strokeWidth={2} dot={false} connectNulls={false} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

function SessionDetailModal({
  open,
  login,
  deviceId,
  onClose,
}: {
  open: boolean;
  login: string;
  deviceId: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const syncedRef = useRef<string | null>(null);

  const lookup = useQuery({
    queryKey: ["bng-session-lookup", deviceId, login],
    enabled: open && !!login && !!deviceId,
    queryFn: () =>
      apiFetch<{
        found: boolean;
        source: string;
        session?: PppoeSession;
        note?: string;
        query: string;
        list_updated?: boolean;
      }>(`/api/v1/bng/devices/${deviceId}/sessions/lookup?q=${encodeURIComponent(login)}`, { timeoutMs: 45_000 }),
    staleTime: 0,
    retry: false,
  });

  const authLogs = useQuery({
    queryKey: ["bng-session-auth", deviceId, login],
    enabled: open && !!login && !!deviceId && lookup.isSuccess,
    queryFn: () =>
      apiFetch<{ auth_attempts: AuthAttemptLog[] }>(
        `/api/v1/bng/devices/${deviceId}/sessions/lookup/auth?q=${encodeURIComponent(login)}`,
        { timeoutMs: 90_000 },
      ),
    staleTime: 0,
    retry: false,
  });

  const sessionIndex = lookup.data?.session?.index?.trim();
  const traffic = useQuery({
    queryKey: ["bng-session-traffic", deviceId, sessionIndex],
    enabled: open && !!sessionIndex && lookup.data?.found === true,
    queryFn: () =>
      apiFetch<TrafficRateSnapshot>(
        `/api/v1/bng/devices/${deviceId}/sessions/traffic-rate?index=${encodeURIComponent(sessionIndex!)}`,
        { timeoutMs: 50_000 },
      ),
    refetchInterval: open && sessionIndex ? 5000 : false,
    staleTime: 0,
    retry: false,
  });

  useEffect(() => {
    if (!open || !lookup.isSuccess || !lookup.data?.found || !lookup.data.session) {
      return;
    }
    const token = `${login}:${lookup.dataUpdatedAt}`;
    if (syncedRef.current === token) {
      return;
    }
    syncedRef.current = token;
    qc.invalidateQueries({ queryKey: ["bng-device-sessions", deviceId] });
    qc.invalidateQueries({ queryKey: ["bng-session-report", deviceId] });
  }, [open, lookup.isSuccess, lookup.data, lookup.dataUpdatedAt, login, deviceId, qc]);

  useEffect(() => {
    if (!open) {
      syncedRef.current = null;
    }
  }, [open]);

  if (!open) return null;

  const sessionLoading = lookup.isLoading || (lookup.isFetching && !lookup.data);
  if (sessionLoading) {
    return (
      <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
        <div
          className="modal"
          role="dialog"
          aria-modal="true"
          aria-busy="true"
          style={{
            maxWidth: 360,
            width: "min(92vw, 360px)",
            padding: "32px 24px",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 12,
          }}
          onMouseDown={(e) => e.stopPropagation()}
        >
          <Loader2 size={36} className="map-refresh-spin" aria-hidden />
          <span style={{ fontSize: 14, textAlign: "center" }}>A consultar login via SNMP…</span>
        </div>
      </div>
    );
  }

  const s = lookup.data?.session;
  const authAttempts = authLogs.data?.auth_attempts ?? [];
  const st = formatBngSessionStatus(s?.status || (lookup.data?.found ? "Up" : "Down"));
  const upRate = traffic.data?.up_display || (traffic.isFetching && !traffic.data ? "…" : "—");
  const dnRate = traffic.data?.dn_display || (traffic.isFetching && !traffic.data ? "…" : "—");

  return (
    <div className="modal-backdrop" role="presentation" onMouseDown={onClose}>
      <div
        className="modal"
        role="dialog"
        aria-modal="true"
        style={{ maxWidth: 920, width: "min(96vw, 920px)", maxHeight: "92vh", overflow: "auto", position: "relative" }}
        onMouseDown={(e) => e.stopPropagation()}
      >
        <h3 style={{ marginTop: 0, marginBottom: 4 }}>Login PPPoE</h3>
        <p className="mono" style={{ fontSize: 13, margin: "0 0 8px" }}>
          {login}
        </p>
        {lookup.isError && (
          <p style={{ fontSize: 12, color: "var(--danger, #dc2626)" }}>{(lookup.error as Error).message}</p>
        )}
        {lookup.data?.note && (
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 6px" }}>{lookup.data.note}</p>
        )}
        <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 12px" }}>
          SNMP em tempo real
          {lookup.data?.found === false ? " · não encontrado online" : ""}
        </p>

        <div style={{ display: "grid", gap: 10, marginBottom: 12 }}>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: 10 }}>
            <SessionDetailSection title="Identificação">
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                <DetailField label="Status" value={st.label} />
                <DetailField label="Login" value={s?.login || login} />
                <DetailField label="Índice" value={s?.index} />
                <DetailField label="Domínio" value={s?.domain} />
              </div>
            </SessionDetailSection>

            <SessionDetailSection title="Rede">
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                <DetailField label="IPv4" value={s?.ipv4} />
                <DetailField label="Tipo IP" value={formatBngIpType(s?.ip_type, s?.ip_type_raw, s)} />
                <DetailField label="IPv6 WAN" value={formatBngIpv6Display(s?.ipv6)} />
                <DetailField label="IPv6 PD/LAN" value={formatBngIpv6Display(s?.ipv6_pd)} />
                <DetailField label="MAC" value={s?.mac} />
                <DetailField label="VLAN" value={s?.vlan} />
                <DetailField label="Interface" value={s?.interface} />
                <DetailField label="Tempo online" value={sessionDisplayOnline(s)} />
              </div>
            </SessionDetailSection>
          </div>

          <SessionDetailSection title="Tráfego em tempo real">
            <p style={{ fontSize: 11, color: "var(--muted)", margin: "0 0 8px" }}>
              Upload/download instantâneos (amostra ~2s, actualiza a cada 5s)
            </p>
            <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
              <div
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  background: "rgba(59,130,246,0.08)",
                  border: "1px solid rgba(59,130,246,0.25)",
                }}
              >
                <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 2 }}>Upload</div>
                <div style={{ fontSize: 20, fontWeight: 700, display: "flex", alignItems: "center", gap: 6 }}>
                  {traffic.isFetching && traffic.data && <Loader2 size={14} className="map-refresh-spin" aria-hidden />}
                  {upRate}
                </div>
              </div>
              <div
                style={{
                  padding: "10px 12px",
                  borderRadius: 8,
                  background: "rgba(63,185,80,0.08)",
                  border: "1px solid rgba(63,185,80,0.25)",
                }}
              >
                <div style={{ fontSize: 11, color: "var(--muted)", marginBottom: 2 }}>Download</div>
                <div style={{ fontSize: 20, fontWeight: 700, display: "flex", alignItems: "center", gap: 6 }}>
                  {traffic.isFetching && traffic.data && <Loader2 size={14} className="map-refresh-spin" aria-hidden />}
                  {dnRate}
                </div>
              </div>
            </div>
          </SessionDetailSection>

          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))", gap: 10 }}>
            <SessionDetailSection title="Limites e QoS">
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                <DetailField label="Limite upstream" value={sessionDisplayUpLimit(s)} />
                <DetailField label="Limite downstream" value={sessionDisplayDnLimit(s)} />
                <DetailField label="Perfil QoS" value={s?.qos_profile} />
              </div>
            </SessionDetailSection>

            <SessionDetailSection title="Estados AAA">
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 8 }}>
                <DetailField label="Autenticação" value={s?.auth_state} />
                <DetailField label="Autorização" value={s?.author_state} />
                <DetailField label="Accounting" value={s?.acct_state} />
              </div>
            </SessionDetailSection>
          </div>
        </div>

        <div className="row" style={{ justifyContent: "flex-end", gap: 8, marginBottom: 12 }}>
          <button type="button" className="btn" onClick={onClose}>
            Fechar
          </button>
          {isAdminUser() && (
            <button
              type="button"
              className="btn btn--primary"
              disabled={lookup.isFetching}
              onClick={() => {
                lookup.refetch();
                authLogs.refetch();
                if (sessionIndex) traffic.refetch();
              }}
            >
              {lookup.isFetching ? "A consultar…" : "Actualizar"}
            </button>
          )}
        </div>

        <SessionDetailSection title="Tentativas de autenticação">
          {authLogs.isLoading && authAttempts.length === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)", margin: 0, display: "flex", alignItems: "center", gap: 6 }}>
              <Loader2 size={14} className="map-refresh-spin" aria-hidden /> A carregar histórico…
            </p>
          ) : authAttempts.length === 0 ? (
            <p style={{ fontSize: 12, color: "var(--muted)", margin: 0 }}>Sem registos recentes.</p>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 4, maxHeight: 130, overflow: "auto" }}>
              {authAttempts.map((a, i) => {
                const ok = a.kind === "success";
                const line = a.message?.trim() || `${a.time || "—"} · ${a.login || login} · ${a.mac || "—"}`;
                return (
                  <div
                    key={`${a.time}-${a.kind}-${i}`}
                    style={{
                      padding: "6px 8px",
                      borderRadius: 4,
                      borderLeft: `2px solid ${ok ? "#3fb950" : "#f85149"}`,
                      background: ok ? "rgba(63,185,80,0.06)" : "rgba(248,81,73,0.06)",
                      fontSize: 11,
                      fontFamily: "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace",
                      lineHeight: 1.4,
                      wordBreak: "break-word",
                    }}
                  >
                    {line}
                  </div>
                );
              })}
            </div>
          )}
        </SessionDetailSection>
      </div>
    </div>
  );
}

export function BngPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast } = useAppToast();
  const [tab, setTab] = useState<BngTab>("overview");
  const [sel, setSel] = useState<string | null>(null);
  const [searchField, setSearchField] = useState<BngSessionSearchField>("login");
  const [searchQuery, setSearchQuery] = useState("");
  const [advancedFilters, setAdvancedFilters] = useState<BngSessionAdvancedFilters>(EMPTY_BNG_SESSION_FILTERS);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [displayLimit, setDisplayLimit] = useState<BngSessionDisplayLimit>(100);
  const [confirmCollectOpen, setConfirmCollectOpen] = useState(false);
  const [detailLogin, setDetailLogin] = useState<string | null>(null);
  const [collectProgress, setCollectProgress] = useState<BngCollectProgress | null>(null);

  const devices = useQuery({
    queryKey: ["bng-devices"],
    queryFn: () => apiFetch<{ devices: BngDevice[] }>("/api/v1/bng/devices"),
  });

  const rows = devices.data?.devices ?? [];
  const selectedId = sel ?? rows[0]?.id ?? null;

  const overview = useQuery({
    queryKey: ["bng-overview", selectedId],
    enabled: !!selectedId,
    queryFn: () => apiFetch<BngOverview>(`/api/v1/bng/devices/${selectedId}/overview`),
    refetchInterval: 60_000,
  });

  const history = useQuery({
    queryKey: ["bng-stats-history", selectedId],
    enabled: !!selectedId && (tab === "relatorio" || tab === "overview"),
    queryFn: () =>
      apiFetch<{ samples: StatsSample[] }>(`/api/v1/bng/stats/history?device_id=${selectedId}&limit=120`),
    refetchInterval: 60_000,
  });

  const sessionReport = useQuery({
    queryKey: ["bng-session-report", selectedId],
    enabled: !!selectedId && tab === "relatorio",
    queryFn: () => apiFetch<SessionReportResponse>(`/api/v1/bng/devices/${selectedId}/sessions/report`),
    refetchInterval: tab === "relatorio" ? 60_000 : false,
  });

  const authRecords = useQuery({
    queryKey: ["bng-auth-records", selectedId],
    enabled: !!selectedId && tab === "auth",
    queryFn: () =>
      apiFetch<{ records: AuthAttemptLog[]; count: number; note?: string; fetched_at?: string }>(
        `/api/v1/bng/devices/${selectedId}/auth-records?limit=50`,
        { timeoutMs: 30_000 },
      ),
    staleTime: 0,
    retry: false,
    refetchInterval: tab === "auth" ? 2000 : false,
    refetchIntervalInBackground: true,
  });

  const sessions = useQuery({
    queryKey: ["bng-device-sessions", selectedId],
    enabled: !!selectedId && tab === "sessions",
    queryFn: () =>
      apiFetch<{
        sessions: PppoeSession[];
        captured_at?: string;
        note?: string;
        count?: number;
      }>(`/api/v1/bng/devices/${selectedId}/sessions`),
  });

  const collectPeriodic = useMutation({
    mutationFn: () => apiFetch(`/api/v1/bng/devices/${selectedId}/collect`, { method: "POST" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["bng-overview", selectedId] });
      qc.invalidateQueries({ queryKey: ["bng-stats-history", selectedId] });
      toastOk(pushToast, "Coleta BNG concluída.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha na coleta."),
  });

  const collectSessions = useMutation({
    mutationFn: async () => {
      if (!selectedId) throw new Error("Equipamento não seleccionado.");
      setConfirmCollectOpen(false);
      setCollectProgress({ status: "running", logins_loaded: 0, message: "A iniciar consulta SNMP…" });
      await apiFetch(`/api/v1/bng/devices/${selectedId}/sessions/collect`, { method: "POST" });
      for (let i = 0; i < 1200; i++) {
        await sleep(1000);
        const st = await apiFetch<BngCollectProgress>(`/api/v1/bng/devices/${selectedId}/sessions/collect/status`);
        setCollectProgress(st);
        if (st.done) {
          if (st.status === "error") throw new Error(st.error || "Falha na consulta SNMP.");
          return st;
        }
      }
      throw new Error("Tempo esgotado na consulta SNMP.");
    },
    onSuccess: (data) => {
      setCollectProgress(null);
      qc.invalidateQueries({ queryKey: ["bng-device-sessions", selectedId] });
      qc.invalidateQueries({ queryKey: ["bng-session-report", selectedId] });
      toastOk(pushToast, `Consulta completa: ${data.session_count ?? 0} sessão(ões) PPPoE.`);
    },
    onError: (err) => {
      setCollectProgress(null);
      toastErr(pushToast, err, "Falha na consulta SNMP.");
    },
  });

  const filteredSessions = useMemo(
    () => filterBngSessions(sessions.data?.sessions ?? [], searchField, searchQuery, advancedFilters),
    [sessions.data?.sessions, searchField, searchQuery, advancedFilters],
  );

  const activeFilterCount = countActiveBngSessionFilters(advancedFilters);

  const displayedSessions = useMemo(
    () => filteredSessions.slice(0, displayLimit),
    [filteredSessions, displayLimit],
  );

  const stats = overview.data?.latest_stats;
  const fields = overview.data?.fields ?? {};
  const historySamples = history.data?.samples ?? [];

  if (devices.isLoading) return <p>A carregar equipamentos BNG…</p>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;

  return (
    <>
      <div className="page-heading">
        <h1>BNG</h1>
        {tab === "sessions" && <PageCountPill label="Sessões" count={displayedSessions.length} />}
      </div>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        Concentradores com switch BNG activo.{" "}
        <Link to={APP_ROUTES.settings}>Configurações → BNG</Link> para OIDs e métricas.
      </p>

      {rows.length === 0 ? (
        <div className="msg msg--warn">
          Nenhum equipamento com BNG activo. Active o switch em{" "}
          <Link to={APP_ROUTES.devices}>Equipamentos</Link>.
        </div>
      ) : (
        <>
          <div className="row" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap", alignItems: "center" }}>
            <label style={{ fontSize: 13 }}>Equipamento</label>
            <select
              className="input"
              style={{ minWidth: 280 }}
              value={selectedId ?? ""}
              onChange={(e) => setSel(e.target.value || null)}
            >
              {rows.map((d) => (
                <option key={d.id} value={d.id}>
                  {d.description || d.ip} {d.ip ? `(${d.ip})` : ""}
                </option>
              ))}
            </select>
            {canMutate && (
              <button
                type="button"
                className="btn"
                disabled={!selectedId || collectPeriodic.isPending}
                onClick={() => collectPeriodic.mutate()}
              >
                <RefreshCw size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                Coletar totais agora
              </button>
            )}
          </div>

          <div className="tabs" style={{ marginBottom: 16 }}>
            <button type="button" className={tab === "overview" ? "active" : ""} onClick={() => setTab("overview")}>
              Visão geral
            </button>
            <button type="button" className={tab === "relatorio" ? "active" : ""} onClick={() => setTab("relatorio")}>
              Relatório
            </button>
            <button type="button" className={tab === "auth" ? "active" : ""} onClick={() => setTab("auth")}>
              Autenticações
            </button>
            <button type="button" className={tab === "sessions" ? "active" : ""} onClick={() => setTab("sessions")}>
              Sessões PPPoE
            </button>
          </div>

          {tab === "overview" && (
            <>
              {stats?.collected_at && (
                <div
                  style={{
                    marginBottom: 12,
                    padding: "8px 12px",
                    borderRadius: 8,
                    background: "var(--surface-2, rgba(255,255,255,0.04))",
                    border: "1px solid var(--border)",
                    display: "inline-flex",
                    flexDirection: "column",
                    gap: 2,
                  }}
                >
                  <span style={{ fontSize: 11, color: "var(--muted)" }}>Última coleta</span>
                  <span style={{ fontSize: 14, fontWeight: 600, fontVariantNumeric: "tabular-nums" }}>
                    {formatBngDateTime(stats.collected_at)}
                  </span>
                </div>
              )}

              <div className="row" style={{ gap: 12, marginBottom: 16, flexWrap: "wrap" }}>
                {[
                  { label: "PPPoE online", value: stats?.pppoe_online },
                  { label: "IPv4 online", value: stats?.ipv4_online },
                  { label: "IPv6 online", value: stats?.ipv6_online },
                  { label: "Dual-stack", value: stats?.dual_stack_online },
                  { label: "Total online", value: stats?.total_online },
                ].map((c) => (
                  <div key={c.label} className="card" style={{ padding: "10px 14px", minWidth: 120 }}>
                    <div style={{ fontSize: 11, color: "var(--muted)" }}>{c.label}</div>
                    <strong style={{ fontSize: 20 }}>{c.value ?? "—"}</strong>
                  </div>
                ))}
              </div>

              <div className="card" style={{ padding: 14 }}>
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "baseline",
                    flexWrap: "wrap",
                    gap: 8,
                    marginBottom: 10,
                  }}
                >
                  <h3 style={{ margin: 0, fontSize: 15 }}>Saúde do equipamento</h3>
                  {stats?.collected_at && (
                    <span style={{ fontSize: 11, color: "var(--muted)" }}>
                      Actualizado: {formatBngDateTime(stats.collected_at)}
                      {overview.data?.telemetry_collected_at &&
                        ` · Telemetria: ${formatBngDateTime(overview.data.telemetry_collected_at)}`}
                    </span>
                  )}
                </div>
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))",
                    gap: 10,
                  }}
                >
                  {OVERVIEW_KEYS.map((key) => {
                    const val = fields[key];
                    if (val == null || val === "") return null;
                    return (
                      <div key={key}>
                        <div style={{ fontSize: 11, color: "var(--muted)" }}>{OVERVIEW_FIELD_LABELS[key]}</div>
                        <div style={{ fontSize: 15, fontWeight: 600 }}>{formatOverviewField(key, val)}</div>
                      </div>
                    );
                  })}
                  {overview.data?.device?.ip && (
                    <div>
                      <div style={{ fontSize: 11, color: "var(--muted)" }}>IP de gestão</div>
                      <div className="mono">{overview.data.device.ip}</div>
                    </div>
                  )}
                </div>
              </div>
            </>
          )}

          {tab === "relatorio" && (
            <>
              <BngSessionReportPanel data={sessionReport.data} loading={sessionReport.isLoading} />

              <h3 style={{ fontSize: 14, margin: "0 0 10px", color: "var(--muted)", fontWeight: 600 }}>
                Totais periódicos (monitoramento)
              </h3>
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                  gap: 12,
                  marginBottom: 16,
                }}
              >
                {STATS_SERIES.map((ser) => (
                  <BngStatsMiniChart
                    key={ser.key}
                    samples={historySamples}
                    seriesKey={ser.key}
                    color={ser.color}
                    label={ser.label}
                  />
                ))}
              </div>

              <div className="card" style={{ padding: 14 }}>
                <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Histórico de coletas</h3>
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>Data/hora</th>
                        <th>PPPoE</th>
                        <th>IPv4</th>
                        <th>IPv6</th>
                        <th>Dual-stack</th>
                        <th>Total</th>
                      </tr>
                    </thead>
                    <tbody>
                      {historySamples.length === 0 ? (
                        <tr>
                          <td colSpan={6} style={{ color: "var(--muted)" }}>
                            Sem amostras ainda. Active totais em Configurações → BNG e aguarde o monitoramento.
                          </td>
                        </tr>
                      ) : (
                        [...historySamples].reverse().map((s) => (
                          <tr key={s.collected_at}>
                            <td style={{ whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
                              {formatBngDateTime(s.collected_at)}
                            </td>
                            <td>{s.pppoe_online ?? "—"}</td>
                            <td>{s.ipv4_online ?? "—"}</td>
                            <td>{s.ipv6_online ?? "—"}</td>
                            <td>{s.dual_stack_online ?? "—"}</td>
                            <td>{s.total_online ?? "—"}</td>
                          </tr>
                        ))
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
            </>
          )}

          {tab === "auth" && (
            <>
              <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 12 }}>
                <button
                  type="button"
                  className="btn btn--sm"
                  disabled={authRecords.isFetching || !selectedId}
                  onClick={() => authRecords.refetch()}
                >
                  <RefreshCw size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                  {authRecords.isFetching ? "A actualizar…" : "Actualizar log AAA"}
                </button>
              </div>
              <BngAuthRecordsPanel
                data={authRecords.data}
                loading={authRecords.isLoading && !authRecords.data}
                refreshing={authRecords.isFetching && !!authRecords.data}
                onLoginClick={(login) => {
                  setTab("sessions");
                  setSearchField("login");
                  setSearchQuery(login);
                  setDetailLogin(login);
                }}
              />
            </>
          )}

          {tab === "sessions" && (
            <>
              <div className="msg msg--warn" style={{ marginBottom: 12, display: "flex", gap: 8, alignItems: "flex-start" }}>
                <AlertTriangle size={18} style={{ flexShrink: 0, marginTop: 2 }} />
                <div style={{ fontSize: 13, lineHeight: 1.5 }}>
                  <strong>Consulta completa via SNMP walk</strong> — percorre milhares de OIDs para listar sessões. A
                  consulta de um login específico vai sempre directo ao equipamento via SNMP (não usa a lista em cache).
                </div>
              </div>

              <div className="row" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
                {canMutate && (
                  <button
                    type="button"
                    className="btn btn--primary"
                    disabled={!selectedId || collectSessions.isPending}
                    onClick={() => setConfirmCollectOpen(true)}
                  >
                    Consulta completa SNMP
                  </button>
                )}
                {sessions.data?.captured_at && (
                  <span style={{ fontSize: 12, color: "var(--muted)", alignSelf: "center" }}>
                    Última consulta: {formatBngDateTime(sessions.data.captured_at)} (
                    {sessions.data.count ?? 0} sessões)
                  </span>
                )}
              </div>

              <div className="card" style={{ padding: 12, marginBottom: 12 }}>
                <div
                  style={{
                    display: "flex",
                    flexWrap: "wrap",
                    gap: 10,
                    alignItems: "flex-end",
                  }}
                >
                  <div className="field" style={{ flex: "0 0 auto", minWidth: 120, margin: 0 }}>
                    <label style={{ fontSize: 11 }}>Pesquisar por</label>
                    <select
                      className="input"
                      value={searchField}
                      onChange={(e) => setSearchField(e.target.value as BngSessionSearchField)}
                    >
                      {BNG_SESSION_SEARCH_FIELDS.map((f) => (
                        <option key={f.value} value={f.value}>
                          {f.label}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="field" style={{ flex: "1 1 180px", minWidth: 160, margin: 0 }}>
                    <label style={{ fontSize: 11 }}>Termo</label>
                    <input
                      className="input mono"
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      placeholder={
                        searchField === "mac"
                          ? "30:16:9d… ou 3016"
                          : searchField === "vlan"
                            ? "100, 200…"
                            : "parcial"
                      }
                    />
                  </div>
                  <div className="field" style={{ flex: "0 0 auto", minWidth: 100, margin: 0 }}>
                    <label style={{ fontSize: 11 }}>Mostrar</label>
                    <select
                      className="input"
                      value={displayLimit}
                      onChange={(e) => setDisplayLimit(Number(e.target.value) as BngSessionDisplayLimit)}
                    >
                      {BNG_SESSION_DISPLAY_LIMITS.map((n) => (
                        <option key={n} value={n}>
                          {n}
                        </option>
                      ))}
                    </select>
                  </div>
                  <button
                    type="button"
                    className={filtersOpen || activeFilterCount > 0 ? "btn btn--primary" : "btn"}
                    style={{ flexShrink: 0 }}
                    onClick={() => setFiltersOpen((v) => !v)}
                  >
                    <Filter size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                    Filtros{activeFilterCount > 0 ? ` (${activeFilterCount})` : ""}
                  </button>
                  <button type="button" className="btn" style={{ flexShrink: 0 }} onClick={() => sessions.refetch()}>
                    <Search size={14} style={{ marginRight: 4, verticalAlign: -2 }} />
                    Actualizar lista
                  </button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    style={{ flexShrink: 0 }}
                    disabled={!searchQuery.trim() || !selectedId || searchField !== "login"}
                    title="Consulta SNMP pontual no equipamento (seleccione Login como campo)"
                    onClick={() => setDetailLogin(searchQuery.trim())}
                  >
                    Consultar login no BNG
                  </button>
                </div>

                {filtersOpen && (
                  <div
                    style={{
                      marginTop: 12,
                      paddingTop: 12,
                      borderTop: "1px solid var(--border)",
                      display: "grid",
                      gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))",
                      gap: 10,
                    }}
                  >
                    <div className="field" style={{ margin: 0 }}>
                      <label style={{ fontSize: 11 }}>IPv4 contém</label>
                      <input
                        className="input mono"
                        value={advancedFilters.ipv4Like}
                        onChange={(e) => setAdvancedFilters((f) => ({ ...f, ipv4Like: e.target.value }))}
                        placeholder="100.64…"
                      />
                    </div>
                    <div className="field" style={{ margin: 0 }}>
                      <label style={{ fontSize: 11 }}>Dual-stack</label>
                      <select
                        className="input"
                        value={advancedFilters.dualStack}
                        onChange={(e) =>
                          setAdvancedFilters((f) => ({
                            ...f,
                            dualStack: e.target.value as BngSessionAdvancedFilters["dualStack"],
                          }))
                        }
                      >
                        <option value="any">Qualquer</option>
                        <option value="yes">Com IPv4 e IPv6</option>
                        <option value="no">Só IPv4 ou só IPv6</option>
                      </select>
                    </div>
                    <div className="field" style={{ margin: 0 }}>
                      <label style={{ fontSize: 11 }}>VLAN(s)</label>
                      <input
                        className="input mono"
                        value={advancedFilters.vlans}
                        onChange={(e) => setAdvancedFilters((f) => ({ ...f, vlans: e.target.value }))}
                        placeholder="100, 200"
                      />
                    </div>
                    <div className="field" style={{ margin: 0 }}>
                      <label style={{ fontSize: 11 }}>Tempo online mín. (s)</label>
                      <input
                        className="input mono"
                        type="number"
                        min={0}
                        value={advancedFilters.minOnlineSec}
                        onChange={(e) => setAdvancedFilters((f) => ({ ...f, minOnlineSec: e.target.value }))}
                        placeholder="3600"
                      />
                    </div>
                    <div className="field" style={{ margin: 0 }}>
                      <label style={{ fontSize: 11 }}>Limite downstream (kbit/s)</label>
                      <input
                        className="input mono"
                        type="number"
                        min={0}
                        value={advancedFilters.dnLimitKbps}
                        onChange={(e) => setAdvancedFilters((f) => ({ ...f, dnLimitKbps: e.target.value }))}
                        placeholder="102400"
                      />
                    </div>
                    <div className="field" style={{ margin: 0, alignSelf: "end" }}>
                      <button
                        type="button"
                        className="btn"
                        onClick={() => setAdvancedFilters(EMPTY_BNG_SESSION_FILTERS)}
                      >
                        Limpar filtros
                      </button>
                    </div>
                  </div>
                )}

                {filteredSessions.length > displayLimit && (
                  <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
                    A mostrar {displayLimit} de {filteredSessions.length} sessão(ões) filtrada(s).
                  </p>
                )}
              </div>

              <div className="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>Status</th>
                      <th>Login</th>
                      <th>IPv4</th>
                      <th>Tipo IP</th>
                      <th>IPv6 WAN</th>
                      <th>IPv6 PD/LAN</th>
                      <th>MAC</th>
                      <th>VLAN</th>
                      <th>Tempo Online</th>
                      <th>Limite up</th>
                      <th>Limite down</th>
                      <th>Autenticação</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {displayedSessions.length === 0 ? (
                      <tr>
                        <td colSpan={13} style={{ color: "var(--muted)" }}>
                          {sessions.isLoading
                            ? "A carregar…"
                            : "Nenhuma sessão. Execute a consulta completa SNMP."}
                        </td>
                      </tr>
                    ) : (
                      displayedSessions.map((s) => {
                        const st = formatBngSessionStatus(s.status);
                        return (
                        <tr key={s.index ?? `${s.login}-${s.mac}`}>
                          <td>
                            <span style={{ color: st.online ? "var(--ok, #16a34a)" : "var(--danger, #dc2626)", fontWeight: 600 }}>
                              {st.label}
                            </span>
                          </td>
                          <td className="mono">{s.login || "—"}</td>
                          <td className="mono">{s.ipv4 || "—"}</td>
                          <td>{formatBngIpType(s.ip_type, s.ip_type_raw, s)}</td>
                          <td className="mono" style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis" }} title={formatBngIpv6Display(s.ipv6)}>
                            {formatBngIpv6Display(s.ipv6)}
                          </td>
                          <td className="mono" style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis" }} title={formatBngIpv6Display(s.ipv6_pd)}>
                            {formatBngIpv6Display(s.ipv6_pd)}
                          </td>
                          <td className="mono">{s.mac || "—"}</td>
                          <td className="mono">{s.vlan || "—"}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayOnline(s)}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayUpLimit(s)}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayDnLimit(s)}</td>
                          <td>{s.auth_state || "—"}</td>
                          <td>
                            {s.login && (
                              <button
                                type="button"
                                className="btn btn--sm"
                                title="Consultar login via SNMP no BNG"
                                onClick={() => setDetailLogin(s.login!)}
                              >
                                <Eye size={14} />
                              </button>
                            )}
                          </td>
                        </tr>
                        );
                      })
                    )}
                  </tbody>
                </table>
              </div>
            </>
          )}
        </>
      )}

      <ConfirmModal
        open={confirmCollectOpen}
        title="Consulta completa PPPoE"
        message="Esta operação executa vários SNMP walks no BNG (login, IP, MAC, IPv6, estados). Em concentradores com milhares de sessões pode demorar vários minutos e aumentar a carga no equipamento. Deseja continuar?"
        confirmLabel="Iniciar consulta"
        danger
        busy={collectSessions.isPending}
        onCancel={() => setConfirmCollectOpen(false)}
        onConfirm={() => collectSessions.mutate()}
      />

      {collectSessions.isPending && (
        <div className="modal-backdrop modal-backdrop--stack" role="presentation" style={{ zIndex: 10070 }}>
          <div className="modal" role="dialog" aria-modal="true" style={{ maxWidth: 420 }} onMouseDown={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Consulta completa PPPoE</h3>
            <p style={{ fontSize: 13, color: "var(--muted)", margin: "0 0 12px" }}>
              {collectProgress?.message || "A consultar sessões via SNMP…"}
            </p>
            <div style={{ marginBottom: 8 }}>
              {collectProgress?.phase === "details" && collectProgress.sessions_total ? (
                <>
                  <div style={{ fontSize: 24, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>
                    {progressLoginLabel(collectProgress.sessions_enriched)} / {collectProgress.sessions_total} sessões
                  </div>
                  <div style={{ fontSize: 11, color: "var(--muted)" }}>
                    Detalhes (IPv4, MAC, IPv6…) por índice — progresso de 500 em 500
                  </div>
                </>
              ) : (
                <>
                  <div style={{ fontSize: 24, fontWeight: 700, fontVariantNumeric: "tabular-nums" }}>
                    {progressLoginLabel(collectProgress?.logins_loaded)} logins
                  </div>
                  <div style={{ fontSize: 11, color: "var(--muted)" }}>Walk de logins (GET-BULK) — actualiza de 50 em 50</div>
                </>
              )}
            </div>
            <div
              style={{
                height: 8,
                borderRadius: 4,
                background: "var(--border)",
                overflow: "hidden",
                marginBottom: 12,
              }}
            >
              <div
                style={{
                  height: "100%",
                  width: `${
                    collectProgress?.phase === "details" && collectProgress.sessions_total
                      ? Math.min(95, Math.max(5, Math.floor(((collectProgress.sessions_enriched ?? 0) / collectProgress.sessions_total) * 100)))
                      : Math.min(95, Math.max(5, Math.floor((collectProgress?.logins_loaded ?? 0) / 500) * 5))
                  }%`,
                  background: "var(--primary, #3b82f6)",
                  transition: "width 0.4s ease",
                }}
              />
            </div>
            {collectProgress?.session_count != null && collectProgress.session_count > 0 && (
              <p style={{ fontSize: 12, margin: 0 }}>{collectProgress.session_count} sessões PPPoE processadas.</p>
            )}
          </div>
        </div>
      )}

      {selectedId && detailLogin && (
        <SessionDetailModal
          open={!!detailLogin}
          login={detailLogin}
          deviceId={selectedId}
          onClose={() => setDetailLogin(null)}
        />
      )}
    </>
  );
}

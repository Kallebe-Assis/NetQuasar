import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { Link } from "react-router-dom";
import {
  BarChart3,
  ChevronLeft,
  ChevronRight,
  Eye,
  Filter,
  KeyRound,
  LayoutDashboard,
  Loader2,
  RefreshCw,
  Users,
} from "lucide-react";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { PageCountPill } from "../components/PageCountPill";
import { ConfirmModal } from "../components/ConfirmModal";
import { BngOverviewPanel } from "../components/BngOverviewPanel";
import { DeviceMonitorShell } from "../components/DeviceMonitorShell";
import "../styles/mikrotik-noc.css";
import { apiFetch } from "../lib/api";
import { isAdminUser } from "../lib/auth";
import {
  BNG_SESSION_DISPLAY_LIMITS,
  BNG_SESSION_REFRESH_MODE_KEY,
  bngCellDisplay,
  formatBngDateTime,
  formatBngIpv6Display,
  formatBngIpType,
  formatBngSessionStatus,
  sessionDisplayDnLimit,
  sessionDisplayOnline,
  sessionDisplayUpLimit,
  STATS_SERIES,
  type BngSessionDisplayLimit,
  type BngSessionRefreshMode,
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

type BngBGPPeer = {
  remote_addr?: string;
  state?: string;
  local_iface?: string;
};

type BngBGP = {
  total_peers?: number;
  established?: number;
  peers?: BngBGPPeer[];
};

type BngPowerSupply = {
  index?: string;
  name?: string;
  description?: string;
  status?: string;
};

type BngPhysicalIface = {
  if_index?: number;
  name?: string;
  oper_status?: string;
  admin_status?: string;
};

type BngPhysicalIfaceSummary = {
  up_count?: number;
  down_count?: number;
  total?: number;
  interfaces?: BngPhysicalIface[];
};

type BngLinkTraffic = {
  if_index?: number;
  name?: string;
  oper_status?: string;
  in_display?: string;
  out_display?: string;
  in_bps?: number;
  out_bps?: number;
};

type BngCGNPublicPool = {
  index?: string;
  instance?: string;
  pool_name?: string;
  start_addr?: string;
  end_addr?: string;
  usage_percent?: number;
};

type BngCGNATMapping = {
  private_ip?: string;
  public_hint?: string;
  pool_name?: string;
  cgnat?: boolean;
  session_count?: number;
};

type BngInfrastructure = {
  collected_at?: string;
  aaa_scalars?: BngAAAScalars;
  power_consumption?: string;
  ipv4_pools?: BngIPPoolRow[];
  ipv6_pools?: BngIPv6PoolRow[];
  radius_servers?: BngRadiusRow[];
  cgn?: BngCGN;
  bgp?: BngBGP;
  power_supplies?: BngPowerSupply[];
  physical_interfaces?: BngPhysicalIfaceSummary;
  link_traffic?: BngLinkTraffic[];
  cgn_public_pools?: BngCGNPublicPool[];
};

type SessionReportResponse = {
  captured_at?: string;
  note?: string;
  session_count?: number;
  report?: SessionBreakdown;
  infrastructure?: BngInfrastructure;
  infrastructure_captured_at?: string;
  infrastructure_note?: string;
  cgnat_summary?: BngCGNATMapping[];
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

const BNG_TABS: Array<{ id: BngTab; label: string; icon: typeof LayoutDashboard }> = [
  { id: "overview", label: "Visão geral", icon: LayoutDashboard },
  { id: "relatorio", label: "Relatório", icon: BarChart3 },
  { id: "auth", label: "Autenticações", icon: KeyRound },
  { id: "sessions", label: "Sessões PPPoE", icon: Users },
];

function mergeLivePppoeSession(cached: PppoeSession, live: PppoeSession): PppoeSession {
  const merged: PppoeSession = { ...cached, ...live };
  const keep = (next?: string, prev?: string) => {
    const n = String(next ?? "").trim();
    if (!n || n === "<nil>" || n.toLowerCase() === "null") return prev;
    return next;
  };
  merged.login = keep(live.login, cached.login);
  merged.index = keep(live.index, cached.index);
  merged.ipv4 = keep(live.ipv4, cached.ipv4);
  merged.mac = keep(live.mac, cached.mac);
  merged.vlan = keep(live.vlan, cached.vlan);
  merged.auth_state = keep(live.auth_state, cached.auth_state);
  return merged;
}

function sessionRangeLabel(page: number, pageSize: number, total: number): string {
  if (total <= 0) return "0/0";
  const start = (page - 1) * pageSize + 1;
  const end = Math.min(page * pageSize, total);
  return `${start}-${end}/${total}`;
}

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

      {infra.bgp && (infra.bgp.total_peers ?? 0) > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>BGP</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 12 }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Total de peers</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{infra.bgp.total_peers ?? "—"}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Established</div>
              <div style={{ fontSize: 18, fontWeight: 600, color: "#3fb950" }}>{infra.bgp.established ?? "—"}</div>
            </div>
          </div>
          {(infra.bgp.peers ?? []).length > 0 && (
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Peer remoto</th>
                    <th>Estado</th>
                  </tr>
                </thead>
                <tbody>
                  {infra.bgp.peers!.map((p) => (
                    <tr key={p.remote_addr}>
                      <td className="mono">{p.remote_addr || "—"}</td>
                      <td style={{ color: p.state === "Established" ? "#3fb950" : undefined }}>{p.state || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {(infra.power_supplies ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Fontes de energia</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Nome</th>
                  <th>Descrição</th>
                  <th>Estado</th>
                </tr>
              </thead>
              <tbody>
                {infra.power_supplies!.map((ps) => (
                  <tr key={ps.index ?? ps.name}>
                    <td className="mono">{ps.name || "—"}</td>
                    <td>{ps.description || "—"}</td>
                    <td style={{ color: ps.status === "UP" || ps.status === "Enabled" ? "#3fb950" : ps.status === "DOWN" ? "#f85149" : undefined }}>
                      {ps.status || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {infra.physical_interfaces && (infra.physical_interfaces.total ?? 0) > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Interfaces físicas</h3>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 12 }}>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>UP</div>
              <div style={{ fontSize: 18, fontWeight: 600, color: "#3fb950" }}>{infra.physical_interfaces.up_count ?? 0}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>DOWN</div>
              <div style={{ fontSize: 18, fontWeight: 600, color: "#f85149" }}>{infra.physical_interfaces.down_count ?? 0}</div>
            </div>
            <div>
              <div style={{ fontSize: 11, color: "var(--muted)" }}>Total</div>
              <div style={{ fontSize: 18, fontWeight: 600 }}>{infra.physical_interfaces.total ?? 0}</div>
            </div>
          </div>
          {(infra.physical_interfaces.interfaces ?? []).length > 0 && (
            <div className="table-wrap" style={{ maxHeight: 280, overflow: "auto" }}>
              <table>
                <thead>
                  <tr>
                    <th>Interface</th>
                    <th>Oper</th>
                    <th>Admin</th>
                  </tr>
                </thead>
                <tbody>
                  {infra.physical_interfaces.interfaces!.map((iface) => (
                    <tr key={`${iface.if_index}-${iface.name}`}>
                      <td className="mono">{iface.name || "—"}</td>
                      <td style={{ color: iface.oper_status === "UP" ? "#3fb950" : iface.oper_status === "DOWN" ? "#f85149" : undefined }}>
                        {iface.oper_status || "—"}
                      </td>
                      <td>{iface.admin_status || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {(infra.link_traffic ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Tráfego por link (uplink BGP)</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Interface</th>
                  <th>Estado</th>
                  <th>Download</th>
                  <th>Upload</th>
                </tr>
              </thead>
              <tbody>
                {infra.link_traffic!.map((l) => (
                  <tr key={l.name ?? l.if_index}>
                    <td className="mono">{l.name || "—"}</td>
                    <td style={{ color: l.oper_status === "UP" ? "#3fb950" : undefined }}>{l.oper_status || "—"}</td>
                    <td className="mono">{l.in_display || (l.in_bps != null ? `${l.in_bps} bps` : "—")}</td>
                    <td className="mono">{l.out_display || (l.out_bps != null ? `${l.out_bps} bps` : "—")}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <p style={{ fontSize: 11, color: "var(--muted)", margin: "8px 0 0" }}>
            Download = tráfego recebido no BNG (in). Upload = tráfego enviado (out). Taxa calculada entre coletas ou amostra de 2 s.
          </p>
        </div>
      )}

      {(infra.cgn_public_pools ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 10px", fontSize: 15 }}>Pools públicos CGNAT</h3>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Pool</th>
                  <th>Instância</th>
                  <th>Range público</th>
                  <th>% uso</th>
                </tr>
              </thead>
              <tbody>
                {infra.cgn_public_pools!.map((p) => (
                  <tr key={p.index ?? `${p.pool_name}-${p.start_addr}`}>
                    <td className="mono">{p.pool_name || "—"}</td>
                    <td>{p.instance || "—"}</td>
                    <td className="mono">
                      {p.start_addr && p.end_addr && p.start_addr !== p.end_addr
                        ? `${p.start_addr} – ${p.end_addr}`
                        : p.start_addr || p.end_addr || "—"}
                    </td>
                    <td>{p.usage_percent != null ? `${p.usage_percent}%` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </>
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

      {(data?.cgnat_summary ?? []).length > 0 && (
        <div className="card" style={{ padding: 14, marginBottom: 16 }}>
          <h3 style={{ margin: "0 0 6px", fontSize: 15 }}>CGNAT — IP privado × pool público</h3>
          <p style={{ margin: "0 0 10px", fontSize: 12, color: "var(--muted)", lineHeight: 1.45 }}>
            O IP por sessão em hwAccessIPAddress é o endereço WAN do cliente (privado em CGNAT). O mapeamento exacto
            privado→público por sessão não existe na HUAWEI-AAA-MIB; abaixo são listados os pools públicos CGNAT
            (HUAWEI-CGN-MIB) associados a cada faixa privada.
          </p>
          <div className="table-wrap" style={{ maxHeight: 360, overflow: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>IP privado / WAN</th>
                  <th>Pool / IP público</th>
                  <th>Pool</th>
                  <th>Sessões</th>
                </tr>
              </thead>
              <tbody>
                {data!.cgnat_summary!.map((row) => (
                  <tr key={row.private_ip}>
                    <td className="mono">{row.private_ip || "—"}</td>
                    <td className="mono">{row.public_hint || "—"}</td>
                    <td>{row.pool_name || (row.cgnat ? "CGNAT" : "—")}</td>
                    <td>{row.session_count?.toLocaleString("pt-PT") ?? "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
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
            formatter={(value) => {
              const n = value == null ? null : Number(value);
              return [n == null || !Number.isFinite(n) ? "—" : n, label];
            }}
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
  const [submittedLoginQuery, setSubmittedLoginQuery] = useState("");
  const [advancedFilters, setAdvancedFilters] = useState<BngSessionAdvancedFilters>(EMPTY_BNG_SESSION_FILTERS);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [displayLimit, setDisplayLimit] = useState<BngSessionDisplayLimit>(10);
  const [sessionPage, setSessionPage] = useState(1);
  const [sessionRefreshMode, setSessionRefreshMode] = useState<BngSessionRefreshMode>(() => {
    try {
      const v = localStorage.getItem(BNG_SESSION_REFRESH_MODE_KEY);
      return v === "auto" ? "auto" : "manual";
    } catch {
      return "manual";
    }
  });
  const [livePageSessions, setLivePageSessions] = useState<PppoeSession[] | null>(null);
  const [livePageLoading, setLivePageLoading] = useState(false);
  const [liveRefreshToken, setLiveRefreshToken] = useState(0);
  const livePageRequestRef = useRef(0);
  const [confirmCollectOpen, setConfirmCollectOpen] = useState(false);
  const [detailLogin, setDetailLogin] = useState<string | null>(null);
  const [collectProgress, setCollectProgress] = useState<BngCollectProgress | null>(null);

  const devices = useQuery({
    queryKey: ["bng-devices"],
    queryFn: () => apiFetch<{ devices: BngDevice[] }>("/api/v1/bng/devices"),
  });

  const rows = devices.data?.devices ?? [];
  const selectedId = sel ?? rows[0]?.id ?? null;

  const [historyDays, setHistoryDays] = useState<1 | 3 | 7 | 30>(7);

  const overview = useQuery({
    queryKey: ["bng-overview", selectedId],
    enabled: !!selectedId,
    placeholderData: keepPreviousData,
    queryFn: () => apiFetch<BngOverview>(`/api/v1/bng/devices/${selectedId}/overview`),
    refetchInterval: 60_000,
  });

  const history = useQuery({
    queryKey: ["bng-stats-history", selectedId, historyDays],
    enabled: !!selectedId && (tab === "relatorio" || tab === "overview"),
    placeholderData: keepPreviousData,
    queryFn: () =>
      apiFetch<{ samples: StatsSample[] }>(
        `/api/v1/bng/stats/history?device_id=${selectedId}&days=${historyDays}`,
      ),
    refetchInterval: 60_000,
  });

  const sessionReport = useQuery({
    queryKey: ["bng-session-report", selectedId],
    enabled: !!selectedId && (tab === "relatorio" || tab === "overview"),
    placeholderData: keepPreviousData,
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

  const loginSearchActive = searchField === "login" && submittedLoginQuery.length >= 2;

  const loginSearch = useQuery({
    queryKey: ["bng-session-login-search", selectedId, submittedLoginQuery],
    enabled: !!selectedId && tab === "sessions" && loginSearchActive,
    queryFn: () =>
      apiFetch<{
        found: boolean;
        source: string;
        session?: PppoeSession;
        note?: string;
        query: string;
      }>(`/api/v1/bng/devices/${selectedId}/sessions/lookup?q=${encodeURIComponent(submittedLoginQuery)}`, {
        timeoutMs: 45_000,
      }),
    staleTime: 0,
    retry: false,
  });

  const cacheFilteredSessions = useMemo(
    () => filterBngSessions(sessions.data?.sessions ?? [], searchField, searchQuery, advancedFilters),
    [sessions.data?.sessions, searchField, searchQuery, advancedFilters],
  );

  const filteredSessions = useMemo(() => {
    if (loginSearchActive) {
      if (loginSearch.data?.found && loginSearch.data.session) {
        return [loginSearch.data.session];
      }
      return [];
    }
    return cacheFilteredSessions;
  }, [loginSearchActive, loginSearch.data, cacheFilteredSessions]);

  const activeFilterCount = countActiveBngSessionFilters(advancedFilters);

  const sessionTotalPages = useMemo(
    () => Math.max(1, Math.ceil(filteredSessions.length / displayLimit)),
    [filteredSessions.length, displayLimit],
  );

  const safeSessionPage = Math.min(sessionPage, sessionTotalPages);

  const pageSlice = useMemo(
    () => filteredSessions.slice((safeSessionPage - 1) * displayLimit, safeSessionPage * displayLimit),
    [filteredSessions, safeSessionPage, displayLimit],
  );

  const pageSliceKey = useMemo(
    () => pageSlice.map((s) => String(s.index ?? s.login ?? "")).join("|"),
    [pageSlice],
  );

  useEffect(() => {
    setSubmittedLoginQuery("");
  }, [selectedId, searchField]);

  useEffect(() => {
    setSessionPage(1);
  }, [selectedId, searchField, searchQuery, advancedFilters, displayLimit]);

  useEffect(() => {
    if (sessionPage > sessionTotalPages) {
      setSessionPage(sessionTotalPages);
    }
  }, [sessionPage, sessionTotalPages]);

  useEffect(() => {
    try {
      localStorage.setItem(BNG_SESSION_REFRESH_MODE_KEY, sessionRefreshMode);
    } catch {
      /* ignore */
    }
  }, [sessionRefreshMode]);

  useEffect(() => {
    if (loginSearchActive || sessionRefreshMode !== "auto" || !selectedId || pageSlice.length === 0) {
      setLivePageSessions(null);
      setLivePageLoading(false);
      return;
    }
    const indices = pageSlice.map((s) => String(s.index ?? "").trim()).filter(Boolean);
    if (indices.length === 0) {
      setLivePageSessions(null);
      return;
    }
    const reqId = ++livePageRequestRef.current;
    setLivePageLoading(true);
    void (async () => {
      try {
        const resp = await apiFetch<{ sessions: PppoeSession[] }>(
          `/api/v1/bng/devices/${selectedId}/sessions/live-batch`,
          {
            method: "POST",
            body: JSON.stringify({ indices }),
            timeoutMs: 90_000,
          },
        );
        if (reqId !== livePageRequestRef.current) return;
        setLivePageSessions(resp.sessions ?? []);
      } catch (err) {
        if (reqId !== livePageRequestRef.current) return;
        setLivePageSessions(null);
        toastErr(pushToast, err, "Falha ao consultar sessões no BNG");
      } finally {
        if (reqId === livePageRequestRef.current) {
          setLivePageLoading(false);
        }
      }
    })();
  }, [loginSearchActive, sessionRefreshMode, selectedId, safeSessionPage, displayLimit, pageSliceKey, liveRefreshToken, pushToast]);

  const displayedSessions = useMemo(() => {
    if (loginSearchActive) {
      if (loginSearch.isFetching) return [];
      return filteredSessions;
    }
    if (sessionRefreshMode === "auto" && livePageSessions && livePageSessions.length > 0) {
      const byIndex = new Map(livePageSessions.map((s) => [String(s.index ?? ""), s]));
      return pageSlice.map((row) => {
        const key = String(row.index ?? "");
        const live = byIndex.get(key);
        return live ? mergeLivePppoeSession(row, live) : row;
      });
    }
    return pageSlice;
  }, [loginSearchActive, loginSearch.isFetching, filteredSessions, sessionRefreshMode, livePageSessions, pageSlice]);

  const sessionTableLoading =
    sessions.isLoading || (loginSearchActive ? loginSearch.isFetching : sessionRefreshMode === "auto" && livePageLoading);

  const stats = overview.data?.latest_stats;
  const fields = overview.data?.fields ?? {};
  const historySamples = history.data?.samples ?? [];
  const selectedDevice = rows.find((d) => d.id === selectedId) ?? null;
  const infra = sessionReport.data?.infrastructure;

  const overviewInitialLoading = !!selectedId && tab === "overview" && !overview.data && overview.isLoading;

  if (devices.isLoading) return <p>A carregar equipamentos BNG…</p>;
  if (devices.isError) return <div className="msg msg--err">{(devices.error as Error).message}</div>;

  const bngToolbar = (
    <>
      <select
        className="input mk-noc-btn"
        style={{ maxWidth: 260, fontSize: 12, padding: "6px 10px" }}
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
          className="mk-noc-btn mk-noc-btn--primary"
          disabled={!selectedId || collectPeriodic.isPending}
          onClick={() => collectPeriodic.mutate()}
        >
          <RefreshCw size={14} className={collectPeriodic.isPending ? "spin" : ""} />
          {collectPeriodic.isPending ? "A coletar…" : "Actualizar telemetria"}
        </button>
      )}
    </>
  );

  return (
    <>
      <style>{`
        .spin { animation: mk-spin 1s linear infinite; }
        @keyframes mk-spin { to { transform: rotate(360deg); } }
      `}</style>
      <div className="page-heading" style={{ marginBottom: 8 }}>
        <h1>BNG</h1>
        {tab === "sessions" && <PageCountPill label="Sessões" count={filteredSessions.length} />}
      </div>

      {rows.length === 0 ? (
        <div className="msg msg--warn">
          Nenhum equipamento com BNG activo. Active o switch em{" "}
          <Link to={APP_ROUTES.devices}>Equipamentos</Link>.
        </div>
      ) : (
        <DeviceMonitorShell
          tabs={BNG_TABS}
          activeTab={tab}
          onTab={setTab}
          title={selectedDevice?.description || selectedDevice?.ip || "BNG"}
          subtitle="Sistema em funcionamento normal"
          online
          meta={
            <>
              <span>
                <strong>IP</strong> <span className="mono">{selectedDevice?.ip || "—"}</span>
              </span>
              <span>
                <strong>PPPoE</strong> {stats?.pppoe_online?.toLocaleString("pt-PT") ?? "—"}
              </span>
              <span>
                <strong>Últ. coleta</strong> {stats?.collected_at ? formatBngDateTime(stats.collected_at) : "—"}
              </span>
            </>
          }
          toolbar={bngToolbar}
        >
          {tab === "overview" && (
            overviewInitialLoading ? (
              <p style={{ color: "var(--muted)", padding: 16 }}>A carregar últimos dados coletados…</p>
            ) : (
            <BngOverviewPanel
              deviceName={selectedDevice?.description || "BNG"}
              deviceIp={selectedDevice?.ip}
              fields={fields}
              stats={stats}
              telemetryCollectedAt={overview.data?.telemetry_collected_at}
              historySamples={historySamples}
              historyDays={historyDays}
              onHistoryDaysChange={setHistoryDays}
              physicalIfaces={infra?.physical_interfaces}
              radiusServers={infra?.radius_servers}
              ipv4Pools={infra?.ipv4_pools}
            />
            )
          )}

          {tab === "relatorio" && (
            <div className="mk-noc-panel" style={{ padding: 14 }}>
              <BngSessionReportPanel data={sessionReport.data} loading={sessionReport.isLoading && !sessionReport.data} />

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
            </div>
          )}

          {tab === "auth" && (
            <>
              <div style={{ display: "flex", justifyContent: "flex-end", marginBottom: 12 }}>
                <button
                  type="button"
                  className="mk-noc-btn"
                  disabled={authRecords.isFetching || !selectedId}
                  onClick={() => authRecords.refetch()}
                >
                  <RefreshCw size={14} className={authRecords.isFetching ? "spin" : ""} />
                  {authRecords.isFetching ? "A actualizar…" : "Actualizar log AAA"}
                </button>
              </div>
              <div className="mk-noc-panel" style={{ padding: 14 }}>
              <BngAuthRecordsPanel
                data={authRecords.data}
                loading={authRecords.isLoading && !authRecords.data}
                refreshing={authRecords.isFetching && !!authRecords.data}
                onLoginClick={(login) => {
                  setTab("sessions");
                  setSearchField("login");
                  setSearchQuery(login);
                }}
              />
              </div>
            </>
          )}

          {tab === "sessions" && (
            <>
              <div className="row" style={{ gap: 8, marginBottom: 12, flexWrap: "wrap" }}>
                {canMutate && (
                  <button
                    type="button"
                    className="mk-noc-btn mk-noc-btn--primary"
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
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && searchField === "login") {
                          e.preventDefault();
                          setSubmittedLoginQuery(searchQuery.trim());
                        }
                      }}
                      placeholder={
                        searchField === "mac"
                          ? "30:16:9d… ou 3016"
                          : searchField === "vlan"
                            ? "100, 200…"
                            : searchField === "login"
                              ? "Enter para pesquisar no equipamento"
                              : "parcial"
                      }
                    />
                  </div>
                  <div className="field" style={{ flex: "0 0 auto", minWidth: 100, margin: 0 }}>
                    <label style={{ fontSize: 11 }}>Por página</label>
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
                  <div className="field" style={{ flex: "0 0 auto", minWidth: 140, margin: 0 }}>
                    <label style={{ fontSize: 11 }}>Dados na tabela</label>
                    <select
                      className="input"
                      value={sessionRefreshMode}
                      onChange={(e) => setSessionRefreshMode(e.target.value as BngSessionRefreshMode)}
                    >
                      <option value="manual">Manual (cache)</option>
                      <option value="auto">Auto-refresh (SNMP live)</option>
                    </select>
                  </div>
                  <button
                    type="button"
                    className={`btn btn--icon-menu${filtersOpen || activeFilterCount > 0 ? " btn--primary" : ""}`}
                    style={{ flexShrink: 0 }}
                    title={activeFilterCount > 0 ? `Filtros (${activeFilterCount})` : "Filtros"}
                    aria-label={activeFilterCount > 0 ? `Filtros (${activeFilterCount})` : "Filtros"}
                    onClick={() => setFiltersOpen((v) => !v)}
                  >
                    <Filter size={16} />
                  </button>
                  <button
                    type="button"
                    className="btn btn--icon-menu"
                    style={{ flexShrink: 0 }}
                    title="Actualizar lista"
                    aria-label="Actualizar lista"
                    disabled={sessionTableLoading}
                    onClick={() => {
                      if (searchField === "login") {
                        setSubmittedLoginQuery(searchQuery.trim());
                        if (submittedLoginQuery === searchQuery.trim()) {
                          void loginSearch.refetch();
                        }
                        return;
                      }
                      if (loginSearchActive) {
                        void loginSearch.refetch();
                        return;
                      }
                      if (sessionRefreshMode === "manual") {
                        void sessions.refetch();
                      } else {
                        setLiveRefreshToken((t) => t + 1);
                      }
                    }}
                  >
                    <RefreshCw size={16} className={sessionTableLoading ? "map-refresh-spin" : undefined} />
                  </button>
                  {!loginSearchActive && filteredSessions.length > 0 ? (
                    <>
                      <button
                        type="button"
                        className="btn btn--icon-menu"
                        disabled={safeSessionPage <= 1 || sessionTableLoading}
                        title="Página anterior"
                        aria-label="Página anterior"
                        onClick={() => setSessionPage((p) => Math.max(1, p - 1))}
                      >
                        <ChevronLeft size={18} />
                      </button>
                      <span
                        className="mono"
                        style={{ fontSize: 11, color: "var(--muted)", minWidth: 52, textAlign: "center" }}
                      >
                        {sessionRangeLabel(safeSessionPage, displayLimit, filteredSessions.length)}
                      </span>
                      <button
                        type="button"
                        className="btn btn--icon-menu"
                        disabled={safeSessionPage >= sessionTotalPages || sessionTableLoading}
                        title="Página seguinte"
                        aria-label="Página seguinte"
                        onClick={() => setSessionPage((p) => Math.min(sessionTotalPages, p + 1))}
                      >
                        <ChevronRight size={18} />
                      </button>
                    </>
                  ) : loginSearchActive && filteredSessions.length > 0 ? (
                    <span className="mono" style={{ fontSize: 11, color: "var(--muted)", minWidth: 52, textAlign: "center" }}>
                      {sessionRangeLabel(1, 1, filteredSessions.length)}
                    </span>
                  ) : null}
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

              </div>

              <div className="table-wrap mk-noc-panel" style={{ position: "relative", minHeight: 220, padding: 0, overflow: "hidden" }}>
                {sessionTableLoading ? (
                  <div
                    style={{
                      position: "absolute",
                      inset: 0,
                      zIndex: 2,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      background: "color-mix(in srgb, var(--panel) 82%, transparent)",
                    }}
                  >
                    <Loader2 size={42} className="map-refresh-spin" aria-label="A carregar sessões" />
                  </div>
                ) : null}
                <table className="mk-noc-table">
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
                    {displayedSessions.length === 0 && !sessionTableLoading ? (
                      <tr>
                        <td colSpan={13} style={{ color: "var(--muted)" }}>
                          {searchField === "login" && submittedLoginQuery.length < 2
                            ? "Digite o login e pressione Enter para pesquisar no equipamento (mín. 2 caracteres)."
                            : loginSearchActive
                            ? loginSearch.isFetching
                              ? "Pesquisando login no equipamento…"
                              : loginSearch.data?.found === false
                                ? "Login não encontrado online no BNG."
                                : "Aguardando resultado da pesquisa."
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
                          <td className="mono">{bngCellDisplay(s.login)}</td>
                          <td className="mono">{bngCellDisplay(s.ipv4)}</td>
                          <td>{formatBngIpType(s.ip_type, s.ip_type_raw, s)}</td>
                          <td className="mono" style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis" }} title={formatBngIpv6Display(s.ipv6)}>
                            {formatBngIpv6Display(s.ipv6)}
                          </td>
                          <td className="mono" style={{ maxWidth: 140, overflow: "hidden", textOverflow: "ellipsis" }} title={formatBngIpv6Display(s.ipv6_pd)}>
                            {formatBngIpv6Display(s.ipv6_pd)}
                          </td>
                          <td className="mono">{bngCellDisplay(s.mac)}</td>
                          <td className="mono">{bngCellDisplay(s.vlan)}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayOnline(s)}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayUpLimit(s)}</td>
                          <td style={{ whiteSpace: "nowrap", fontSize: 12 }}>{sessionDisplayDnLimit(s)}</td>
                          <td>{bngCellDisplay(s.auth_state)}</td>
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
        </DeviceMonitorShell>
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

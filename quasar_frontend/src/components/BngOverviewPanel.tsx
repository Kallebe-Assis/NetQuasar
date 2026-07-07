import { useMemo } from "react";
import {
  Activity,
  Cpu,
  Globe,
  MemoryStick,
  Network,
  Server,
  Thermometer,
  Timer,
  Users,
  Wifi,
  Zap,
} from "lucide-react";
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { KpiCard, parsePercentValue, RingGauge } from "./DeviceMonitorWidgets";
import { EM_DASH } from "../lib/formatDisplay";
import { formatBngDateTime, formatBngTemperature, formatOverviewField, type OverviewFieldKey } from "../lib/bngDisplay";

type StatsSample = {
  collected_at: string;
  total_online?: number | null;
  pppoe_online?: number | null;
  ipv4_online?: number | null;
  ipv6_online?: number | null;
  dual_stack_online?: number | null;
};

const HISTORY_DAY_OPTIONS = [
  { value: 1 as const, label: "Hoje" },
  { value: 3 as const, label: "3 dias" },
  { value: 7 as const, label: "7 dias" },
  { value: 30 as const, label: "30 dias" },
];

type Props = {
  deviceName: string;
  deviceIp?: string | null;
  fields?: Record<string, string | number>;
  stats?: {
    collected_at?: string;
    total_online?: number | null;
    pppoe_online?: number | null;
    ipv4_online?: number | null;
    ipv6_online?: number | null;
    dual_stack_online?: number | null;
  };
  telemetryCollectedAt?: string;
  historySamples: StatsSample[];
  historyDays: 1 | 3 | 7 | 30;
  onHistoryDaysChange: (days: 1 | 3 | 7 | 30) => void;
  physicalIfaces?: { up_count?: number; down_count?: number; total?: number };
  radiusServers?: Array<{ ip?: string; type?: string; responses?: string }>;
  ipv4Pools?: Array<{ name?: string; used_percent?: number; used_ips?: number; total_ips?: number }>;
};

function formatStat(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(Number(v))) return EM_DASH;
  return Number(v).toLocaleString("pt-PT");
}

function parseBngTemperatureC(raw: unknown): number | null {
  if (raw == null || raw === "") return null;
  const n = Number(String(raw).replace(",", "."));
  if (!Number.isFinite(n)) return null;
  if (n > 200) return Math.round(n / 10);
  return Math.round(n * 10) / 10;
}

function formatHistoryAxisTime(iso: string, days: number): string {
  const d = new Date(iso);
  if (days === 1) {
    return d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
}

export function BngOverviewPanel(props: Props) {
  const cpuPct = parsePercentValue(props.fields?.cpu_usage);
  const memPct = parsePercentValue(props.fields?.memory_usage);
  const tempRaw = props.fields?.temperature;
  const tempC = parseBngTemperatureC(tempRaw);
  const uptime = formatOverviewField("sys_uptime" as OverviewFieldKey, props.fields?.sys_uptime);

  const trafficChart = useMemo(() => {
    const sorted = [...props.historySamples].sort(
      (a, b) => new Date(a.collected_at).getTime() - new Date(b.collected_at).getTime(),
    );
    return sorted.map((p) => ({
      t: formatHistoryAxisTime(p.collected_at, props.historyDays),
      pppoe: p.pppoe_online ?? 0,
      total: p.total_online ?? 0,
    }));
  }, [props.historySamples, props.historyDays]);

  const topPools = (props.ipv4Pools ?? [])
    .filter((p) => (p.used_percent ?? 0) > 0 || (p.used_ips ?? 0) > 0)
    .sort((a, b) => (b.used_percent ?? 0) - (a.used_percent ?? 0))
    .slice(0, 5);

  const radiusServers = props.radiusServers ?? [];

  return (
    <>
      <div className="mk-noc-kpi-row" style={{ gridTemplateColumns: "repeat(6, minmax(0, 1fr))" }}>
        <KpiCard icon={Wifi} title="PPPoE online">
          <div className="mk-noc-kpi__value">{formatStat(props.stats?.pppoe_online)}</div>
        </KpiCard>
        <KpiCard icon={Globe} title="IPv4 online">
          <div className="mk-noc-kpi__value">{formatStat(props.stats?.ipv4_online)}</div>
        </KpiCard>
        <KpiCard icon={Globe} title="IPv6 online">
          <div className="mk-noc-kpi__value">{formatStat(props.stats?.ipv6_online)}</div>
        </KpiCard>
        <KpiCard icon={Users} title="Dual-stack">
          <div className="mk-noc-kpi__value">{formatStat(props.stats?.dual_stack_online)}</div>
        </KpiCard>
        <KpiCard icon={Activity} title="Total online">
          <div className="mk-noc-kpi__value">{formatStat(props.stats?.total_online)}</div>
        </KpiCard>
        <KpiCard icon={Network} title="Interfaces físicas">
          <div className="mk-noc-kpi__value mk-noc-kpi__value--sm">
            {props.physicalIfaces
              ? `${props.physicalIfaces.up_count ?? 0} / ${props.physicalIfaces.total ?? 0} UP`
              : EM_DASH}
          </div>
        </KpiCard>
      </div>

      <div className="mk-noc-kpi-row" style={{ gridTemplateColumns: "repeat(5, minmax(0, 1fr))" }}>
        <KpiCard icon={Cpu} title="CPU">
          <RingGauge pct={cpuPct} label="CPU" color="var(--mk-cpu)" />
        </KpiCard>
        <KpiCard icon={MemoryStick} title="Memória">
          <RingGauge pct={memPct} label="RAM" color="var(--accent)" />
        </KpiCard>
        <KpiCard icon={Thermometer} title="Temperatura">
          <div className="mk-noc-kpi__value mk-noc-kpi__value--ok">
            {tempC != null ? `${tempC} °C` : formatBngTemperature(tempRaw)}
          </div>
        </KpiCard>
        <KpiCard icon={Timer} title="Uptime">
          <div className="mk-noc-kpi__value mk-noc-kpi__value--sm">{uptime}</div>
        </KpiCard>
        <KpiCard icon={Server} title={`RADIUS (${radiusServers.length})`}>
          {radiusServers.length === 0 ? (
            <div className="mk-noc-kpi__value mk-noc-kpi__value--sm">{EM_DASH}</div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 4, fontSize: 11, lineHeight: 1.35 }}>
              {radiusServers.map((r, i) => (
                <div key={`${r.ip ?? "r"}-${i}`}>
                  <span className="mono">{r.ip || EM_DASH}</span>
                  {r.type ? <span className="mk-noc-muted"> · {r.type}</span> : null}
                </div>
              ))}
            </div>
          )}
        </KpiCard>
      </div>

      <div className="mk-noc-mid">
        <div className="mk-noc-panel">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8, marginBottom: 8 }}>
            <h3 style={{ margin: 0 }}>
              <Activity size={14} />
              Sessões online (histórico)
            </h3>
            <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
              {HISTORY_DAY_OPTIONS.map((opt) => (
                <button
                  key={opt.value}
                  type="button"
                  className={`btn btn--sm${props.historyDays === opt.value ? " btn--primary" : ""}`}
                  onClick={() => props.onHistoryDaysChange(opt.value)}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </div>
          <div style={{ height: 200 }}>
            {trafficChart.length >= 2 ? (
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={trafficChart}>
                  <defs>
                    <linearGradient id="bngPppoe" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="0%" stopColor="var(--ok)" stopOpacity={0.35} />
                      <stop offset="100%" stopColor="var(--ok)" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid stroke="var(--mk-chart-grid)" vertical={false} />
                  <XAxis dataKey="t" tick={{ fill: "var(--muted)", fontSize: 10 }} axisLine={false} tickLine={false} />
                  <YAxis tick={{ fill: "var(--muted)", fontSize: 10 }} axisLine={false} tickLine={false} />
                  <Tooltip
                    contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", color: "var(--text)", fontSize: 11 }}
                  />
                  <Area type="monotone" dataKey="pppoe" stroke="var(--ok)" fill="url(#bngPppoe)" name="PPPoE" />
                  <Area type="monotone" dataKey="total" stroke="var(--accent)" fill="none" name="Total" />
                </AreaChart>
              </ResponsiveContainer>
            ) : (
              <p className="mk-noc-muted">Aguardando histórico de coletas periódicas.</p>
            )}
          </div>
        </div>

        <div className="mk-noc-panel">
          <h3>
            <Zap size={14} />
            Pools IPv4 (top uso)
          </h3>
          {topPools.length === 0 ? (
            <p className="mk-noc-muted">Execute consulta completa SNMP para carregar pools.</p>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {topPools.map((p) => (
                <div key={p.name}>
                  <div style={{ display: "flex", justifyContent: "space-between", fontSize: 11, marginBottom: 3 }}>
                    <span className="mono">{p.name}</span>
                    <strong>{p.used_percent != null ? `${p.used_percent}%` : EM_DASH}</strong>
                  </div>
                  <div
                    style={{
                      height: 6,
                      borderRadius: 999,
                      background: "var(--panel2)",
                      overflow: "hidden",
                    }}
                  >
                    <div
                      style={{
                        height: "100%",
                        width: `${Math.min(100, Math.max(0, p.used_percent ?? 0))}%`,
                        background: "var(--accent)",
                        borderRadius: 999,
                      }}
                    />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="mk-noc-panel">
          <h3>
            <Server size={14} />
            Equipamento
          </h3>
          <div className="mk-noc-sys-grid" style={{ gridTemplateColumns: "1fr" }}>
            <div>
              <span>Nome</span>
              <span>{props.deviceName}</span>
            </div>
            <div>
              <span>IP gestão</span>
              <span className="mono">{props.deviceIp || EM_DASH}</span>
            </div>
            <div>
              <span>CPU / Memória</span>
              <span>
                {cpuPct != null ? `${Math.round(cpuPct)}%` : EM_DASH} / {memPct != null ? `${Math.round(memPct)}%` : EM_DASH}
              </span>
            </div>
            <div>
              <span>Uptime / Temp.</span>
              <span>
                {uptime} / {tempC != null ? `${tempC} °C` : formatBngTemperature(tempRaw)}
              </span>
            </div>
            <div>
              <span>Última coleta</span>
              <span>{formatBngDateTime(props.stats?.collected_at)}</span>
            </div>
            <div>
              <span>Telemetria</span>
              <span>{formatBngDateTime(props.telemetryCollectedAt)}</span>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

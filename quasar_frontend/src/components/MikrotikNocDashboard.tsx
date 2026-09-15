import { useMemo } from "react";
import {
  Activity,
  Cable,
  Cpu,
  HardDrive,
  LayoutDashboard,
  MemoryStick,
  Network,
  RefreshCw,
  Server,
  Settings,
  Thermometer,
  Timer,
  Wifi,
  Zap,
} from "lucide-react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { Link } from "react-router-dom";
import { DeviceMonitorShell } from "./DeviceMonitorShell";
import { KpiCard, RingGauge } from "./DeviceMonitorWidgets";
import { EM_DASH, formatSnmpDisplayText } from "../lib/formatDisplay";
import { formatBitrate } from "../lib/formatBitrate";
import {
  buildMikrotikNocKpis,
  buildMikrotikSfpPanels,
  buildMikrotikSystemInfo,
  ifDisplayName,
  ifOperUp,
  inferIfType,
  type MikrotikIfRow,
} from "../lib/mikrotikNocData";
import {
  buildSwitchNocKpis,
  buildSwitchSfpPanels,
  buildSwitchSystemInfo,
} from "../lib/switchNocData";

export type MikrotikNocSection = "overview" | "interfaces" | "pppoe" | "sfp" | "sistema";

type Props = {
  section: MikrotikNocSection;
  onSection: (s: MikrotikNocSection) => void;
  deviceLabel: string;
  deviceModel?: string | null;
  deviceIp?: string | null;
  deviceOnline?: boolean;
  monitorSubtitle?: string;
  variant?: "mikrotik" | "switch";
  softwareLabel?: string;
  collectedAt?: string;
  formatCollectedAt: (iso?: string) => string;
  metrics?: Record<string, unknown>;
  ifaces: MikrotikIfRow[];
  ifaceCollectedAt?: string;
  cpuHistory: Array<{ ts: number; v: number }>;
  memHistory: Array<{ ts: number; v: number }>;
  canMutate: boolean;
  collecting: boolean;
  refreshingIf: boolean;
  onCollect: () => void;
  onRefreshIf: () => void;
  telnetProfileSelect?: React.ReactNode;
  collectionWarning?: React.ReactNode;
  dataFreshnessWarning?: React.ReactNode;
  interfacesPanel?: React.ReactNode;
  pppoeSessions?: Array<Record<string, unknown>>;
  pppoeKnownLogins?: number;
  pppoeLoading?: boolean;
  pppoeError?: string;
  pppoeCollecting?: boolean;
  onPppoeCollect?: () => void;
};

const NAV: Array<{ id: MikrotikNocSection; label: string; icon: typeof LayoutDashboard }> = [
  { id: "overview", label: "Visão Geral", icon: LayoutDashboard },
  { id: "interfaces", label: "Interfaces", icon: Network },
  { id: "pppoe", label: "PPPoE", icon: Wifi },
  { id: "sfp", label: "SFP / Óptico", icon: Cable },
  { id: "sistema", label: "Sistema", icon: Server },
];


export function MikrotikNocDashboard(props: Props) {
  const isSwitch = props.variant === "switch";
  const nav = useMemo(
    () => (isSwitch ? NAV.filter((t) => t.id !== "pppoe") : NAV),
    [isSwitch],
  );
  const softwareLabel = props.softwareLabel ?? (isSwitch ? "Software" : "RouterOS");
  const kpis = useMemo(
    () => (isSwitch ? buildSwitchNocKpis(props.metrics, props.deviceLabel) : buildMikrotikNocKpis(props.metrics, props.deviceLabel)),
    [isSwitch, props.metrics, props.deviceLabel],
  );
  const sys = useMemo(
    () => (isSwitch ? buildSwitchSystemInfo(props.metrics, kpis) : buildMikrotikSystemInfo(props.metrics, kpis)),
    [isSwitch, props.metrics, kpis],
  );
  const sfpPanels = useMemo(
    () => (isSwitch ? buildSwitchSfpPanels(props.metrics, props.ifaces) : buildMikrotikSfpPanels(props.metrics, props.ifaces)),
    [isSwitch, props.metrics, props.ifaces],
  );

  const cpuChart = props.cpuHistory.map((p) => ({
    t: new Date(p.ts).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    v: p.v,
  }));
  const memChart = props.memHistory.map((p) => ({
    t: new Date(p.ts).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit", second: "2-digit" }),
    v: p.v,
  }));

  const ifacePreview = props.ifaces.slice(0, 12);

  const toolbar = (
    <>
      {props.telnetProfileSelect}
      {props.canMutate ? (
        <>
          <button type="button" className="mk-noc-btn mk-noc-btn--primary" disabled={props.collecting} onClick={props.onCollect}>
            <RefreshCw size={14} className={props.collecting ? "spin" : ""} />
            {props.collecting ? "A coletar…" : "Atualizar telemetria"}
          </button>
          {props.section === "interfaces" ? (
            <button type="button" className="mk-noc-btn" disabled={props.refreshingIf} onClick={props.onRefreshIf}>
              <Activity size={14} className={props.refreshingIf ? "spin" : ""} />
              {props.refreshingIf ? "A atualizar…" : "Atualizar interfaces"}
            </button>
          ) : (
            <button type="button" className="mk-noc-btn" disabled={props.refreshingIf} onClick={props.onRefreshIf}>
              <Activity size={14} />
              Interfaces SNMP
            </button>
          )}
        </>
      ) : null}
      <Link to="/settings" className="mk-noc-btn" style={{ textDecoration: "none" }}>
        <Settings size={14} />
        Config.
      </Link>
    </>
  );

  return (
    <DeviceMonitorShell
      tabs={nav}
      activeTab={props.section}
      onTab={props.onSection}
      title={kpis.identity}
      subtitle={props.monitorSubtitle ?? (isSwitch ? "Monitor Switch" : "Monitor RouterOS")}
      online={props.deviceOnline !== false}
      meta={
        <>
          <span>
            <strong>Modelo</strong> {formatSnmpDisplayText(props.deviceModel || sys.model)}
          </span>
          <span>
            <strong>IP</strong> <span className="mono">{props.deviceIp || EM_DASH}</span>
          </span>
          <span>
            <strong>{softwareLabel}</strong> {sys.version}
          </span>
          <span>
            <strong>Últ. atualização</strong> {props.formatCollectedAt(props.collectedAt)}
          </span>
        </>
      }
      toolbar={toolbar}
    >
      {props.dataFreshnessWarning}
      {props.collectionWarning}

      {props.section === "overview" && (
            <>
              <div className="mk-noc-kpi-row">
                <KpiCard icon={Timer} title="Uptime">
                  <div className="mk-noc-kpi__value mk-noc-kpi__value--sm">{kpis.uptime}</div>
                </KpiCard>
                <KpiCard icon={Cpu} title="CPU">
                  <RingGauge pct={kpis.cpuPct} label="CPU" sub={kpis.cpuFreq ?? undefined} color="var(--mk-cpu)" />
                </KpiCard>
                <KpiCard icon={Thermometer} title="Temperatura CPU">
                  <div className="mk-noc-kpi__value mk-noc-kpi__value--ok">
                    {kpis.tempCpu ?? EM_DASH}
                  </div>
                </KpiCard>
                <KpiCard icon={MemoryStick} title="Memória">
                  <RingGauge pct={kpis.memPct} label="RAM" sub={kpis.memFree !== EM_DASH ? `${kpis.memFree} livre` : undefined} color="var(--accent)" />
                </KpiCard>
                <KpiCard icon={HardDrive} title="Disco">
                  <RingGauge pct={kpis.diskPct} label="Disco" sub={kpis.diskFree !== EM_DASH ? `${kpis.diskFree} livre` : undefined} color="var(--mk-disk)" />
                </KpiCard>
                <KpiCard icon={Thermometer} title="Temperatura">
                  <div className="mk-noc-kpi__value mk-noc-kpi__value--ok">
                    {kpis.tempC != null ? `${kpis.tempC.toFixed(0)} °C` : EM_DASH}
                  </div>
                  {kpis.tempBoard ? <div className="mk-noc-kpi__sub">Placa {kpis.tempBoard}</div> : null}
                </KpiCard>
                <KpiCard icon={Zap} title="Voltagem">
                  <div className="mk-noc-kpi__value mk-noc-kpi__value--ok">
                    {kpis.voltageV != null ? `${kpis.voltageV.toFixed(1)} V` : EM_DASH}
                  </div>
                </KpiCard>
              </div>

              <div className="mk-noc-mid">
                <div className="mk-noc-panel">
                  <h3>
                    <Activity size={14} />
                    CPU
                  </h3>
                  <div className="mk-noc-chart-mini">
                    {cpuChart.length >= 2 ? (
                      <ResponsiveContainer width="100%" height="100%">
                        <LineChart data={cpuChart} margin={{ top: 4, right: 8, bottom: 0, left: -20 }}>
                          <CartesianGrid stroke="var(--mk-chart-grid)" vertical={false} />
                          <XAxis dataKey="t" tick={{ fill: "var(--muted)", fontSize: 9 }} axisLine={false} tickLine={false} minTickGap={24} />
                          <YAxis tick={{ fill: "var(--muted)", fontSize: 9 }} axisLine={false} tickLine={false} unit="%" domain={[0, 100]} />
                          <Line type="monotone" dataKey="v" stroke="var(--mk-cpu)" dot={false} strokeWidth={2} name="CPU %" />
                          <Tooltip contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", color: "var(--text)", fontSize: 11 }} />
                        </LineChart>
                      </ResponsiveContainer>
                    ) : (
                      <p className="mk-noc-muted">CPU actual: {kpis.cpuPct != null ? `${kpis.cpuPct.toFixed(0)}%` : EM_DASH}</p>
                    )}
                  </div>
                </div>

                <div className="mk-noc-panel">
                  <h3>
                    <MemoryStick size={14} />
                    Memória
                  </h3>
                  <div className="mk-noc-chart-mini">
                    {memChart.length >= 2 ? (
                      <ResponsiveContainer width="100%" height="100%">
                        <LineChart data={memChart} margin={{ top: 4, right: 8, bottom: 0, left: -20 }}>
                          <CartesianGrid stroke="var(--mk-chart-grid)" vertical={false} />
                          <XAxis dataKey="t" tick={{ fill: "var(--muted)", fontSize: 9 }} axisLine={false} tickLine={false} minTickGap={24} />
                          <YAxis tick={{ fill: "var(--muted)", fontSize: 9 }} axisLine={false} tickLine={false} unit="%" domain={[0, 100]} />
                          <Line type="monotone" dataKey="v" stroke="var(--mk-mem)" dot={false} strokeWidth={2} name="RAM %" />
                          <Tooltip contentStyle={{ background: "var(--panel)", border: "1px solid var(--border)", color: "var(--text)", fontSize: 11 }} />
                        </LineChart>
                      </ResponsiveContainer>
                    ) : (
                      <p className="mk-noc-muted">RAM: {kpis.memPct != null ? `${kpis.memPct.toFixed(0)}%` : EM_DASH}</p>
                    )}
                  </div>
                </div>
              </div>

              <div className="mk-noc-bottom">
                <div className="mk-noc-panel">
                  <h3>
                    <Network size={14} />
                    Interfaces
                  </h3>
                  <div style={{ overflowX: "auto" }}>
                    <table className="mk-noc-table">
                      <thead>
                        <tr>
                          <th />
                          <th>Interface</th>
                          <th>Tipo</th>
                          <th>Estado</th>
                          <th>MTU</th>
                          <th>Download</th>
                          <th>Upload</th>
                          <th>RX dBm</th>
                          <th>TX dBm</th>
                        </tr>
                      </thead>
                      <tbody>
                        {ifacePreview.map((r) => (
                          <tr key={r.if_index}>
                            <td>
                              <span className={`mk-noc-dot ${ifOperUp(r) ? "mk-noc-dot--up" : "mk-noc-dot--down"}`} />
                            </td>
                            <td>{ifDisplayName(r)}</td>
                            <td>{inferIfType(r)}</td>
                            <td>{ifOperUp(r) ? "up" : "down"}</td>
                            <td className="mono">{r.mtu ?? 1500}</td>
                            <td className="mono">{formatBitrate(r.in_bps)}</td>
                            <td className="mono">{formatBitrate(r.out_bps)}</td>
                            <td className="mono">{r.rx_dbm != null ? `${r.rx_dbm}` : EM_DASH}</td>
                            <td className="mono">{r.tx_dbm != null ? `${r.tx_dbm}` : EM_DASH}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                  {props.ifaces.length > ifacePreview.length ? (
                    <button type="button" className="mk-noc-btn" style={{ marginTop: 10 }} onClick={() => props.onSection("interfaces")}>
                      Ver todas ({props.ifaces.length})
                    </button>
                  ) : null}
                </div>

                <div className="mk-noc-panel">
                  <h3>
                    <Server size={14} />
                    Informações do sistema
                  </h3>
                  <div className="mk-noc-sys-grid">
                    <div>
                      <span>Modelo</span>
                      <span>{sys.model}</span>
                    </div>
                    <div>
                      <span>Arquitectura</span>
                      <span>{sys.arch}</span>
                    </div>
                    <div>
                      <span>CPU</span>
                      <span>{sys.cpu}</span>
                    </div>
                    <div>
                      <span>Frequência</span>
                      <span>{sys.cpuFreq}</span>
                    </div>
                    <div>
                      <span>Memória total</span>
                      <span>{sys.memTotal}</span>
                    </div>
                    <div>
                      <span>Memória livre</span>
                      <span>{sys.memFree}</span>
                    </div>
                    <div>
                      <span>{softwareLabel}</span>
                      <span>{sys.version}</span>
                    </div>
                    <div>
                      <span>Plataforma</span>
                      <span>{sys.platform}</span>
                    </div>
                    <div>
                      <span>Board</span>
                      <span>{sys.board}</span>
                    </div>
                    <div>
                      <span>Uptime</span>
                      <span>{sys.uptime}</span>
                    </div>
                  </div>
                </div>
              </div>
            </>
          )}

          {!isSwitch && props.section === "pppoe" && (
            <div className="mk-noc-panel">
              <div className="row" style={{ justifyContent: "space-between", alignItems: "flex-start", flexWrap: "wrap", gap: 8 }}>
                <div>
                  <h2 className="mk-noc-section-title">Sessões PPPoE activas</h2>
                  <p className="mk-noc-muted">
                    {(props.pppoeSessions ?? []).length} sessão(ões) ao vivo por telnet (/ppp active)
                    {props.pppoeKnownLogins ? ` — ${props.pppoeKnownLogins} login(s) conhecido(s)` : ""}
                  </p>
                </div>
                {props.canMutate && props.onPppoeCollect ? (
                  <button type="button" className="btn btn--sm" disabled={props.pppoeCollecting} onClick={props.onPppoeCollect}>
                    <RefreshCw size={13} /> {props.pppoeCollecting ? "A consultar…" : "Consulta completa"}
                  </button>
                ) : null}
              </div>

              {props.pppoeError ? (
                <p className="mk-noc-muted" style={{ color: "var(--err)", marginTop: 10 }}>
                  Falha ao consultar sessões PPPoE por telnet: {props.pppoeError}
                </p>
              ) : props.pppoeLoading && !props.pppoeSessions ? (
                <p className="mk-noc-muted" style={{ marginTop: 10 }}>A consultar sessões PPPoE ao vivo…</p>
              ) : (props.pppoeSessions ?? []).length === 0 ? (
                <p className="mk-noc-muted" style={{ marginTop: 10 }}>Nenhuma sessão PPPoE activa agora.</p>
              ) : (
                <table className="mk-noc-table" style={{ marginTop: 12 }}>
                  <thead>
                    <tr>
                      <th>Login</th>
                      <th>IPv4</th>
                      <th>MAC</th>
                      <th>Tempo online</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(props.pppoeSessions ?? []).map((s, i) => (
                      <tr key={String(s.login ?? s.index ?? i)}>
                        <td>{String(s.login ?? EM_DASH)}</td>
                        <td className="mono">{String(s.ipv4 ?? EM_DASH) || EM_DASH}</td>
                        <td className="mono">{String(s.mac ?? EM_DASH) || EM_DASH}</td>
                        <td>{String(s.online_time ?? EM_DASH) || EM_DASH}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          )}

          {props.section === "sfp" && (
            <div className="mk-noc-sfp-grid">
              {sfpPanels.length === 0 ? (
                <div className="mk-noc-panel">
                  <p className="mk-noc-muted">Nenhum módulo SFP detectado. Active coleta óptica no perfil SNMP/telnet.</p>
                </div>
              ) : (
                sfpPanels.map((s) => (
                  <div key={s.name} className="mk-noc-panel">
                    <h3>
                      <Cable size={14} />
                      {s.name}
                    </h3>
                    <div className="mk-noc-sfp-card">
                      <h4>
                        {s.name}{" "}
                        <span className={`mk-noc-badge ${s.online ? "mk-noc-badge--on" : "mk-noc-badge--off"}`} style={{ fontSize: 9 }}>
                          {s.online ? "ONLINE" : "DOWN"}
                        </span>
                      </h4>
                      {(
                        [
                          ["Temperatura", s.temperatureC],
                          ["Voltagem", s.voltageV],
                          ["TX Bias", s.txBiasMa],
                          ["TX Power", s.txDbm],
                          ["RX Power", s.rxDbm],
                          ["Fabricante", s.vendor],
                          ["Modelo", s.model],
                          ["Serial", s.serial],
                        ] as const
                      ).map(([k, v]) => (
                        <div key={k} className="mk-noc-sfp-row">
                          <span>{k}</span>
                          <strong>{v}</strong>
                        </div>
                      ))}
                    </div>
                  </div>
                ))
              )}
            </div>
          )}

          {props.section === "sistema" && (
            <div className="mk-noc-panel">
              <h2 className="mk-noc-section-title">Sistema</h2>
              <div className="mk-noc-sys-grid" style={{ maxWidth: 560 }}>
                {Object.entries(sys).map(([k, v]) => (
                  <div key={k}>
                    <span>{k}</span>
                    <span>{v}</span>
                  </div>
                ))}
              </div>
            </div>
          )}

          {props.section === "interfaces" && props.interfacesPanel}
    </DeviceMonitorShell>
  );
}

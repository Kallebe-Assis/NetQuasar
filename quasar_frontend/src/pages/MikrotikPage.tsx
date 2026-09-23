import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { PageCountPill } from "../components/PageCountPill";
import { MikrotikNocDashboard, type MikrotikNocSection } from "../components/MikrotikNocDashboard";
import { apiFetch } from "../lib/api";
import { EM_DASH, formatDbm, formatSnmpDisplayText } from "../lib/formatDisplay";
import { formatBitrate } from "../lib/formatBitrate";
import { can, isAdminUser } from "../lib/auth";
import { DropdownMenu } from "../components/DropdownMenu";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { collectDeviceTelemetry } from "../lib/telemetryCollectToast";
import { parseMikrotikCollectionStatus } from "../lib/deviceReportHelpers";
import { formatRelativeCompactPt } from "../lib/alertsPresentation";
import { buildMikrotikNocKpis, ifDisplayName, type MikrotikIfRow } from "../lib/mikrotikNocData";
import { queryKeys } from "../lib/queryKeys";
import { isDeviceOnline, trafficHistoryFromInterfaces, useInterfaceMonitorLoop } from "../lib/monitor";

type DeviceRow = {
  id: string;
  description?: string | null;
  category?: string | null;
  brand?: string | null;
  model?: string | null;
  ip?: string | null;
  telemetry_enabled?: boolean;
  mikrotik_telnet_profile_id?: string | null;
};

type SensorRow = { oid?: string; value?: string; type?: string };

type MikrotikOverviewRow = {
  id: string;
  description?: string | null;
  ip?: string | null;
  locality_name?: string | null;
  online?: boolean | null;
  snmp_health_status?: string | null;
  metrics?: Record<string, unknown> | null;
  telemetry_collected_at?: string | null;
};

function MikrotikOverviewTable({
  rows,
  loading,
  onSelect,
}: {
  rows: MikrotikOverviewRow[];
  loading: boolean;
  onSelect: (id: string) => void;
}) {
  if (loading) return <p style={{ color: "var(--muted)" }}>A carregar equipamentos…</p>;
  if (rows.length === 0) return <p style={{ color: "var(--muted)" }}>Nenhum equipamento MikroTik cadastrado.</p>;
  return (
    <div className="table-wrap" style={{ maxWidth: "100%", overflowX: "auto" }}>
    <table style={{ fontSize: 12 }}>
      <thead>
        <tr>
          <th>Descrição</th>
          <th>IP</th>
          <th>Localidade</th>
          <th>CPU</th>
          <th>Memória</th>
          <th>Uptime</th>
          <th>Estado</th>
          <th>Última coleta</th>
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => {
          const kpis = buildMikrotikNocKpis(r.metrics ?? undefined, r.description ?? "");
          return (
            <tr key={r.id} onClick={() => onSelect(r.id)} style={{ cursor: "pointer" }}>
              <td>{r.description ?? EM_DASH}</td>
              <td className="mono">{r.ip ?? EM_DASH}</td>
              <td>{r.locality_name ?? EM_DASH}</td>
              <td>{kpis.cpuPct != null ? `${kpis.cpuPct.toFixed(0)}%` : EM_DASH}</td>
              <td>{kpis.memPct != null ? `${kpis.memPct.toFixed(0)}%` : EM_DASH}</td>
              <td>{kpis.uptime}</td>
              <td>
                {r.online === true ? (
                  <span className="db-status-pill db-status-pill--ok">Online</span>
                ) : r.online === false ? (
                  <span className="db-status-pill db-status-pill--err">Offline</span>
                ) : (
                  <span className="db-status-pill db-status-pill--muted">—</span>
                )}
              </td>
              <td style={{ color: "var(--muted)", fontSize: 12 }}>{formatRelativeCompactPt(r.telemetry_collected_at)}</td>
            </tr>
          );
        })}
      </tbody>
    </table>
    </div>
  );
}

function isMikrotik(d: DeviceRow): boolean {
  if (String(d.category ?? "").trim().toLowerCase() === "switch") return false;
  const c = String(d.category ?? "").toLowerCase();
  const b = String(d.brand ?? "").toLowerCase();
  return c.includes("mikrotik") || b.includes("mikrotik");
}

function inferIfaceType(r: MikrotikIfRow): string {
  const custom = String(r.custom_type ?? "").trim().toLowerCase();
  if (custom === "sfp") return "SFP";
  if (custom === "ether") return "Ether";
  const n = String(r.if_name ?? r.display_name ?? r.descr ?? "").toLowerCase();
  if (n.includes("wlan") || n.includes("wifi")) return "Wireless";
  if (n.includes("sfp")) return "SFP";
  if (n.includes("bridge")) return "Bridge";
  if (n.includes("pppoe")) return "PPPoE";
  if (n.includes("vlan")) return "VLAN";
  return "Ether";
}

function interfaceDescription(r: MikrotikIfRow): string {
  const custom = String(r.custom_description ?? "").trim();
  return custom || EM_DASH;
}

function ifaceStatus(r: MikrotikIfRow): "up" | "down" | "other" {
  const s = String(r.oper_status ?? "").toLowerCase();
  if (s === "up") return "up";
  if (s === "down") return "down";
  return "other";
}

function MiniTrafficChart({ points }: { points: Array<{ ts: number; tx: number; rx: number }> }) {
  if (points.length < 2) {
    const p = points[0];
    return (
      <div>
        <p className="mk-noc-muted" style={{ margin: "0 0 6px 0" }}>Aguardando histórico do gráfico em tempo real.</p>
        <div className="row" style={{ gap: 12 }}>
          <span className="mono">TX atual: {p ? formatBitrate(p.tx) : "—"}</span>
          <span className="mono">RX atual: {p ? formatBitrate(p.rx) : "—"}</span>
        </div>
      </div>
    );
  }
  const w = 520;
  const h = 170;
  const pad = 16;
  const maxV = Math.max(1, ...points.map((p) => (Number.isFinite(p.tx) ? p.tx : 0)), ...points.map((p) => (Number.isFinite(p.rx) ? p.rx : 0)));
  const xFor = (i: number) => pad + (i * (w - pad * 2)) / Math.max(1, points.length - 1);
  const yFor = (v: number) => h - pad - (Math.max(0, v) / maxV) * (h - pad * 2);
  const txPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.tx)}`).join(" ");
  const rxPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.rx)}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label="Gráfico de tráfego da interface">
      <rect x={0} y={0} width={w} height={h} fill="transparent" />
      <line x1={pad} y1={h - pad} x2={w - pad} y2={h - pad} stroke="var(--border)" strokeWidth={1} />
      <path d={txPath} fill="none" stroke="var(--ok)" strokeWidth={2} />
      <path d={rxPath} fill="none" stroke="var(--accent)" strokeWidth={2} />
    </svg>
  );
}

export function MikrotikPage() {
  const canMutate = isAdminUser() || can("mikrotik.collect") || can("devices.collect");
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const [telCollecting, setTelCollecting] = useState(false);
  const [sel, setSel] = useState<string | null>(null);
  const [section, setSection] = useState<MikrotikNocSection>("overview");
  const [realtimeOn, setRealtimeOn] = useState(false);
  const [realtimeMs, setRealtimeMs] = useState(3000);
  const [realtimeModalOpen, setRealtimeModalOpen] = useState(false);
  const [realtimeDraft, setRealtimeDraft] = useState("3000");
  const [liveTable, setLiveTable] = useState<MikrotikIfRow[]>([]);
  const [selectedChartIfs, setSelectedChartIfs] = useState<number[]>([]);
  const [trafficHistory, setTrafficHistory] = useState<Record<number, Array<{ ts: number; tx: number; rx: number }>>>({});
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "up" | "down">("all");
  const [typeFilter, setTypeFilter] = useState<"all" | "Ether" | "Wireless" | "SFP" | "Bridge" | "PPPoE" | "VLAN">("all");
  const [trafficFilter] = useState<"all" | "with" | "without">("all");
  const chartsRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!canMutate && realtimeOn) setRealtimeOn(false);
  }, [canMutate, realtimeOn]);

  const devices = useQuery({
    queryKey: ["devices-mikrotik-list"],
    queryFn: () => apiFetch<{ devices: DeviceRow[] }>("/api/v1/devices"),
  });

  const rows = useMemo(() => (devices.data?.devices ?? []).filter(isMikrotik), [devices.data?.devices]);
  const selectedDevice = useMemo(() => rows.find((r) => r.id === sel) ?? null, [rows, sel]);

  const monitoring = useQuery({
    queryKey: queryKeys.monitoringActiveEquipment,
    queryFn: () =>
      apiFetch<{ devices: Array<{ id: string; ping_reachable?: boolean | null }> }>(
        "/api/v1/monitoring/active-equipment",
      ),
    refetchInterval: 15_000,
  });

  const deviceOnline = useMemo(() => {
    if (!sel) return false;
    const row = monitoring.data?.devices?.find((d) => d.id === sel);
    return isDeviceOnline(row);
  }, [monitoring.data, sel]);

  const iface = useQuery({
    queryKey: ["mikrotik-if", sel],
    enabled: !!sel,
    staleTime: 30_000,
    // Refresco automático moderado — antes só actualizava no carregamento inicial, no botão
    // manual "Atualizar interfaces" ou com "Tempo real" ligado, ficando visualmente "presa" o
    // resto do tempo (relatado: "só as interfaces... que também precisam de revisão").
    refetchInterval: sel ? 60_000 : false,
    queryFn: () =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: MikrotikIfRow[];
        interface_count?: number;
        walk_truncated?: boolean;
        walk_note?: string;
        optical_sensors?: SensorRow[];
        note?: string;
      }>(`/api/v1/interfaces/devices/${sel}`),
  });

  const telemetry = useQuery({
    queryKey: ["mikrotik-tel", sel],
    enabled: !!sel,
    staleTime: 30_000,
    refetchInterval: sel ? 30_000 : false,
    queryFn: () => apiFetch<{ collected_at?: string; metrics?: Record<string, unknown>; note?: string }>(`/api/v1/telemetry/devices/${sel}/latest`),
  });

  // Histórico de CPU/memória "das últimas coletas" — amostras já persistidas em
  // telemetry_samples, não acumulação ao vivo (que começava sempre vazia a cada troca de
  // equipamento/recarregamento de página e só desenhava algo depois de ficar minutos com a aba
  // aberta). Mostra logo ao abrir a aba, com dados reais das últimas coletas.
  const telemetryHistoryQ = useQuery({
    queryKey: ["mikrotik-tel-history", sel],
    enabled: !!sel,
    staleTime: 30_000,
    queryFn: () =>
      apiFetch<{ samples: Array<{ collected_at: string; metrics?: Record<string, unknown> }> }>(
        `/api/v1/telemetry/history?device_id=${sel}&limit=60`,
      ),
  });

  const cpuHistory = useMemo(() => {
    const samples = telemetryHistoryQ.data?.samples ?? [];
    const out: Array<{ ts: number; v: number }> = [];
    for (const s of [...samples].reverse()) {
      const kpis = buildMikrotikNocKpis(s.metrics, "");
      if (kpis.cpuPct != null) out.push({ ts: new Date(s.collected_at).getTime(), v: kpis.cpuPct });
    }
    return out;
  }, [telemetryHistoryQ.data?.samples]);

  const memHistory = useMemo(() => {
    const samples = telemetryHistoryQ.data?.samples ?? [];
    const out: Array<{ ts: number; v: number }> = [];
    for (const s of [...samples].reverse()) {
      const kpis = buildMikrotikNocKpis(s.metrics, "");
      if (kpis.memPct != null) out.push({ ts: new Date(s.collected_at).getTime(), v: kpis.memPct });
    }
    return out;
  }, [telemetryHistoryQ.data?.samples]);

  const telnetProfiles = useQuery({
    queryKey: ["mikrotik-telnet-profiles"],
    queryFn: () => apiFetch<{ profiles: Array<{ id: string; name: string; is_default?: boolean }> }>("/api/v1/settings/mikrotik-telnet-profiles"),
    staleTime: 60_000,
  });

  // Sessões PPPoE por telnet (/ppp active) — carregado só quando a aba PPPoE está aberta (a
  // consulta ao vivo demora alguns segundos por MikroTik, não vale a pena pedir sempre).
  const pppoeSessions = useQuery({
    queryKey: ["mikrotik-pppoe-sessions", sel],
    enabled: !!sel && section === "pppoe",
    staleTime: 20_000,
    queryFn: () =>
      apiFetch<{ sessions: Array<Record<string, unknown>>; count: number; known_logins?: number }>(
        `/api/v1/mikrotik/devices/${sel}/pppoe-sessions`,
      ),
  });

  const pppoeCollect = useMutation({
    mutationFn: (id: string) => apiFetch(`/api/v1/mikrotik/devices/${id}/pppoe-sessions/collect`, { method: "POST" }),
    onSuccess: () => {
      toastOk(pushToast, "Consulta PPPoE iniciada — a lista actualiza em instantes.");
      setTimeout(() => void qc.invalidateQueries({ queryKey: ["mikrotik-pppoe-sessions", sel] }), 4000);
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao iniciar consulta PPPoE."),
  });

  const patchTelnetProfile = useMutation({
    mutationFn: ({ deviceId, profileId }: { deviceId: string; profileId: string | null }) =>
      apiFetch(`/api/v1/devices/${deviceId}`, { method: "PATCH", json: { mikrotik_telnet_profile_id: profileId } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["devices-mikrotik-list"] });
      toastOk(pushToast, "Perfil telnet atualizado.");
    },
    onError: (err) => toastErr(pushToast, err, "Falha ao atribuir perfil."),
  });

  const refreshIf = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: MikrotikIfRow[];
        interface_count?: number;
        walk_truncated?: boolean;
        walk_note?: string;
        pppoe_session_count?: number;
      }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} }),
    onSuccess: (data, id) => {
      if (data?.device_id) qc.setQueryData(["mikrotik-if", id], data);
      qc.invalidateQueries({ queryKey: ["mikrotik-if", id] });
      // O backend deriva as sessões PPPoE do mesmo walk IF-MIB deste refresh.
      qc.invalidateQueries({ queryKey: ["mikrotik-tel", id] });
      const n = data.interface_count ?? data.interface_table?.length;
      const ppp = data.pppoe_session_count;
      const base = typeof n === "number" ? `Interfaces atualizadas (${n}).` : "Interfaces atualizadas.";
      toastOk(pushToast, typeof ppp === "number" ? `${base} PPPoE: ${ppp} sessão(ões).` : base);
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao atualizar interfaces."),
  });

  const realtimeTick = useMutation({
    mutationFn: async (id: string) => {
      try {
        return await apiFetch<{ updates?: Array<{ if_index: number; tx_dbm?: number; rx_dbm?: number; in_bps?: number; out_bps?: number }> }>(
          `/api/v1/interfaces/devices/${id}/realtime`,
          { method: "POST", json: {} },
        );
      } catch (e) {
        const msg = String((e as Error)?.message ?? e);
        if (!msg.includes("404") && !msg.toLowerCase().includes("not found")) throw e;
        const full = await apiFetch<{ interface_table?: MikrotikIfRow[] }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} });
        return {
          updates: (full.interface_table ?? []).map((r) => ({
            if_index: r.if_index,
            tx_dbm: r.tx_dbm,
            rx_dbm: r.rx_dbm,
            in_bps: r.in_bps,
            out_bps: r.out_bps,
          })),
        };
      }
    },
    onSuccess: (data) => {
      const ups = data?.updates ?? [];
      if (ups.length === 0) return;
      setLiveTable((prev) =>
        prev.map((row) => {
          const up = ups.find((u) => u.if_index === row.if_index);
          if (!up) return row;
          return { ...row, tx_dbm: up.tx_dbm ?? row.tx_dbm, rx_dbm: up.rx_dbm ?? row.rx_dbm, in_bps: up.in_bps ?? row.in_bps, out_bps: up.out_bps ?? row.out_bps };
        }),
      );
    },
    onError: (e) => toastErr(pushToast, e, "Falha na atualização em tempo real."),
  });

  const runTelCollect = async () => {
    if (!sel || !selectedDevice) return;
    setTelCollecting(true);
    try {
      await collectDeviceTelemetry(sel, selectedDevice.description ?? "", { push: pushToast, dismiss: dismissToast }, qc);
      await qc.invalidateQueries({ queryKey: ["mikrotik-tel", sel] });
    } finally {
      setTelCollecting(false);
    }
  };

  const table = liveTable;
  const walkTruncated = Boolean(iface.data?.walk_truncated) || /truncad/i.test(String(iface.data?.walk_note ?? iface.data?.note ?? ""));
  const collectionStatus = useMemo(
    () => parseMikrotikCollectionStatus(telemetry.data?.metrics as Record<string, unknown> | undefined),
    [telemetry.data?.metrics],
  );

  // Distingue "nunca coletado" / "desactualizado" / "sincronização PPPoE falhou" de "valor
  // genuinamente zero" — antes, qualquer um destes casos mostrava só "—" nos cartões, sem
  // nenhuma explicação (relatado: "não me mostra uptime, cpu, memória, nada").
  const dataFreshness = useMemo(() => {
    if (!sel || telemetry.isLoading) return null;
    if (!telemetry.data || telemetry.data.note === "sem telemetria persistida") {
      return { tone: "err" as const, text: "Nunca foi coletada telemetria para este equipamento." };
    }
    const collectedAt = telemetry.data.collected_at ? new Date(telemetry.data.collected_at) : null;
    if (collectedAt && Number.isFinite(collectedAt.getTime())) {
      const ageMin = (Date.now() - collectedAt.getTime()) / 60_000;
      if (ageMin > 15) {
        const ageLabel = ageMin > 120 ? `${Math.round(ageMin / 60)} h` : `${Math.round(ageMin)} min`;
        return { tone: "warn" as const, text: `Última coleta há ${ageLabel} — os valores abaixo podem estar desactualizados.` };
      }
    }
    const metrics = telemetry.data.metrics as Record<string, unknown> | undefined;
    const pppoeErr = metrics?.mikrotik_pppoe_sync_error;
    if (typeof pppoeErr === "string" && pppoeErr) {
      return { tone: "warn" as const, text: `Sincronização de sessões PPPoE por telnet falhou: ${pppoeErr}` };
    }
    return null;
  }, [sel, telemetry.isLoading, telemetry.data]);

  const interfaceRowsFiltered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return table.filter((r) => {
      const type = inferIfaceType(r);
      const status = ifaceStatus(r);
      const tx = Number(r.out_bps ?? 0);
      const rx = Number(r.in_bps ?? 0);
      const hasTraffic = Number.isFinite(tx) && Number.isFinite(rx) && (tx > 0 || rx > 0);
      if (statusFilter !== "all" && status !== statusFilter) return false;
      if (typeFilter !== "all" && type !== typeFilter) return false;
      if (trafficFilter === "with" && !hasTraffic) return false;
      if (trafficFilter === "without" && hasTraffic) return false;
      if (!q) return true;
      const hay = `${r.if_index} ${r.display_name ?? ""} ${r.if_name ?? ""} ${r.descr ?? ""} ${r.custom_description ?? ""} ${type}`.toLowerCase();
      return hay.includes(q);
    });
  }, [table, search, statusFilter, typeFilter, trafficFilter]);

  const selectedChartRows = useMemo(() => table.filter((r) => selectedChartIfs.includes(r.if_index)), [table, selectedChartIfs]);

  useEffect(() => {
    setLiveTable(iface.data?.interface_table ?? []);
  }, [iface.data?.interface_table, sel]);

  useEffect(() => {
    const now = Date.now();
    setTrafficHistory((prev) => trafficHistoryFromInterfaces(prev, table, now));
  }, [table]);

  useInterfaceMonitorLoop({
    deviceId: sel,
    queryKey: ["mikrotik-if", sel ?? ""],
    canMutate,
    onTable: (rows) => setLiveTable(rows as MikrotikIfRow[]),
    enabled: !!sel && !realtimeOn,
    snmpAutoRefresh: false,
  });

  useEffect(() => {
    if (!realtimeOn || !sel) return;
    const intervalMs = Math.max(1500, Number(realtimeMs) || 3000);
    const timer = window.setInterval(() => {
      realtimeTick.mutate(sel);
    }, intervalMs);
    return () => window.clearInterval(timer);
  }, [realtimeOn, realtimeMs, sel, realtimeTick.mutate]);

  useEffect(() => {
    setRealtimeOn(false);
    setSelectedChartIfs([]);
    setTrafficHistory({});
  }, [sel]);

  const overviewQ = useQuery({
    queryKey: ["mikrotik-devices-overview"],
    queryFn: () => apiFetch<{ mikrotiks: MikrotikOverviewRow[] }>("/api/v1/mikrotik/devices"),
    enabled: !sel,
    refetchInterval: sel ? false : 30_000,
  });

  const initialDataLoading =
    !!sel && !iface.data && !telemetry.data && (iface.isLoading || telemetry.isLoading);

  const devicePicker = (
    <select
      className="input"
      style={{ maxWidth: 280, fontSize: 12 }}
      value={sel ?? ""}
      onChange={(e) => setSel(e.target.value || null)}
    >
      {rows.map((d) => (
        <option key={d.id} value={d.id}>
          {d.description ?? "—"}
        </option>
      ))}
    </select>
  );

  const telnetProfileSelect =
    canMutate && sel ? (
      <select
        className="input"
        style={{ maxWidth: 180, fontSize: 12 }}
        value={selectedDevice?.mikrotik_telnet_profile_id ?? ""}
        disabled={patchTelnetProfile.isPending}
        onChange={(e) => patchTelnetProfile.mutate({ deviceId: sel, profileId: e.target.value === "" ? null : e.target.value })}
      >
        <option value="">Telnet: Padrão</option>
        {(telnetProfiles.data?.profiles ?? []).map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
    ) : null;

  const collectionWarning =
    collectionStatus && (collectionStatus.missingOid.length > 0 || collectionStatus.message) ? (
      <div className="msg msg--warn" style={{ fontSize: 12, marginBottom: 12 }}>
        {collectionStatus.message || "Algumas métricas activas não têm OID configurado."}
        {collectionStatus.missingOid.length > 0 ? (
          <span>
            {" "}
            Campos: <span className="mono">{collectionStatus.missingOid.join(", ")}</span>
          </span>
        ) : null}
      </div>
    ) : null;

  const dataFreshnessWarning = dataFreshness ? (
    <div className={`msg msg--${dataFreshness.tone}`} style={{ fontSize: 12, marginBottom: 12 }}>
      {dataFreshness.text}
    </div>
  ) : null;

  const interfacesPanel = (
    <div className="mk-noc-panel" style={{ background: "transparent", border: "none", padding: 0 }}>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 12 }}>
        {devicePicker}
        <input className="input" style={{ minWidth: 200, fontSize: 12 }} placeholder="Buscar interface…" value={search} onChange={(e) => setSearch(e.target.value)} />
        <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as "all" | "up" | "down")}>
          <option value="all">Todos status</option>
          <option value="up">UP</option>
          <option value="down">DOWN</option>
        </select>
        <select className="input" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as typeof typeFilter)}>
          <option value="all">Todos tipos</option>
          <option value="Ether">Ether</option>
          <option value="Wireless">Wireless</option>
          <option value="SFP">SFP</option>
          <option value="Bridge">Bridge</option>
          <option value="PPPoE">PPPoE</option>
          <option value="VLAN">VLAN</option>
        </select>
        {canMutate ? (
          <>
            <button type="button" className="mk-noc-btn" disabled={refreshIf.isPending || !sel} onClick={() => sel && refreshIf.mutate(sel)}>
              {refreshIf.isPending ? "A atualizar…" : "Atualizar interfaces"}
            </button>
            <button
              type="button"
              className="mk-noc-btn"
              onClick={() => {
                if (realtimeOn) {
                  setRealtimeOn(false);
                  return;
                }
                setRealtimeDraft(String(Math.max(1500, realtimeMs || 3000)));
                setRealtimeModalOpen(true);
              }}
              disabled={!sel}
            >
              {realtimeOn ? "Parar tempo real" : "Tempo real"}
            </button>
          </>
        ) : null}
        <DropdownMenu
          align="end"
          minWidth={210}
          trigger={({ toggle }) => (
            <button type="button" className="mk-noc-btn" onClick={toggle}>
              ⚙
            </button>
          )}
        >
          {({ close }) => (
            <Link to="/devices" className="mk-options-item" onClick={() => close()} style={{ color: "inherit" }}>
              Ir para equipamentos ↗
            </Link>
          )}
        </DropdownMenu>
      </div>
      {walkTruncated && (
        <div className="msg msg--warn" style={{ fontSize: 12, marginBottom: 8 }}>
          Coleta SNMP truncada — aumente o timeout em Configurações → Alertas.
        </div>
      )}
      {realtimeOn && <p style={{ fontSize: 11, color: "var(--ok)" }}>Tempo real activo ({realtimeMs} ms)</p>}
      <div className="table-wrap" style={{ maxHeight: "min(70vh, 640px)", overflow: "auto" }}>
        <table className="mk-noc-table">
          <thead>
            <tr>
              <th>Idx</th>
              <th>Nome</th>
              <th>Descrição</th>
              <th>Tipo</th>
              <th>Status</th>
              <th>TX</th>
              <th>RX</th>
              <th>TX dBm</th>
              <th>RX dBm</th>
              <th>Gráfico</th>
            </tr>
          </thead>
          <tbody>
            {interfaceRowsFiltered.map((r) => (
              <tr key={r.if_index}>
                <td className="mono">{r.if_index}</td>
                <td>{ifDisplayName(r)}</td>
                <td>{interfaceDescription(r)}</td>
                <td>{inferIfaceType(r)}</td>
                <td>
                  <span className={`mk-noc-dot ${ifaceStatus(r) === "up" ? "mk-noc-dot--up" : "mk-noc-dot--down"}`} /> {ifaceStatus(r)}
                </td>
                <td className="mono">{formatBitrate(r.out_bps)}</td>
                <td className="mono">{formatBitrate(r.in_bps)}</td>
                <td className="mono">{formatDbm(r.tx_dbm)}</td>
                <td className="mono">{formatDbm(r.rx_dbm)}</td>
                <td>
                  <input
                    type="checkbox"
                    checked={selectedChartIfs.includes(r.if_index)}
                    onChange={(e) => {
                      if (e.target.checked) setSelectedChartIfs((p) => [...p, r.if_index]);
                      else setSelectedChartIfs((p) => p.filter((x) => x !== r.if_index));
                    }}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {selectedChartRows.length > 0 && (
        <div ref={chartsRef} style={{ marginTop: 14, display: "grid", gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))", gap: 10 }}>
          {selectedChartRows.map((row) => (
            <div key={row.if_index} className="mk-noc-panel">
              <strong style={{ fontSize: 12 }}>{ifDisplayName(row)}</strong>
              <MiniTrafficChart points={trafficHistory[row.if_index] ?? []} />
            </div>
          ))}
        </div>
      )}
    </div>
  );

  return (
    <>
      <style>{`
        .mk-switch { position: relative; width: 36px; height: 20px; border-radius: 999px; border: 1px solid var(--border); background: var(--panel2); display: inline-block; }
        .mk-switch__knob { position: absolute; top: 1px; left: 1px; width: 16px; height: 16px; border-radius: 999px; background: var(--toggle-thumb-active); transition: transform 140ms; }
        .mk-switch--on { background: color-mix(in srgb, var(--ok) 45%, var(--panel2)); }
        .mk-switch--on .mk-switch__knob { transform: translateX(16px); }
        .mk-options-item { display: flex; padding: 8px 10px; text-decoration: none; border-radius: 6px; }
        .mk-options-item:hover { background: var(--hover-bg-menu); }
        @keyframes mk-spin { to { transform: rotate(360deg); } }
        .spin { animation: mk-spin 1s linear infinite; }
      `}</style>

      {devices.isLoading && <p>A carregar equipamentos…</p>}
      {devices.isError && <div className="msg msg--err">{(devices.error as Error).message}</div>}

      {!devices.isLoading && !devices.isError && (
        <>
          <div className="page-heading" style={{ marginBottom: 8 }}>
            <h1>MikroTik</h1>
            <PageCountPill label="Mikrotiks" count={rows.length} />
          </div>
          {rows.length === 0 ? (
            <p style={{ color: "var(--muted)" }}>Nenhum equipamento MikroTik cadastrado.</p>
          ) : !sel ? (
            <MikrotikOverviewTable rows={overviewQ.data?.mikrotiks ?? []} loading={overviewQ.isLoading} onSelect={setSel} />
          ) : selectedDevice ? (
            initialDataLoading ? (
              <p style={{ color: "var(--muted)" }}>A carregar últimos dados coletados…</p>
            ) : (
            <>
            <button type="button" className="btn" style={{ marginBottom: 12 }} onClick={() => setSel(null)}>
              ← Todos os MikroTiks
            </button>
            <MikrotikNocDashboard
              section={section}
              onSection={setSection}
              deviceLabel={selectedDevice.description ?? EM_DASH}
              deviceModel={formatSnmpDisplayText(selectedDevice.model)}
              deviceIp={selectedDevice.ip}
              deviceOnline={deviceOnline}
              collectedAt={telemetry.data?.collected_at}
              formatCollectedAt={formatRelativeCompactPt}
              metrics={telemetry.data?.metrics}
              ifaces={table}
              ifaceCollectedAt={iface.data?.collected_at}
              cpuHistory={cpuHistory}
              memHistory={memHistory}
              canMutate={canMutate}
              collecting={telCollecting}
              refreshingIf={refreshIf.isPending}
              onCollect={() => void runTelCollect()}
              onRefreshIf={() => sel && refreshIf.mutate(sel)}
              telnetProfileSelect={
                <>
                  {rows.length > 1 ? devicePicker : null}
                  {telnetProfileSelect}
                </>
              }
              collectionWarning={collectionWarning}
              dataFreshnessWarning={dataFreshnessWarning}
              interfacesPanel={interfacesPanel}
              pppoeSessions={pppoeSessions.data?.sessions}
              pppoeKnownLogins={pppoeSessions.data?.known_logins}
              pppoeLoading={pppoeSessions.isLoading || pppoeSessions.isFetching}
              pppoeError={pppoeSessions.isError ? (pppoeSessions.error as Error)?.message : undefined}
              pppoeCollecting={pppoeCollect.isPending}
              onPppoeCollect={() => sel && pppoeCollect.mutate(sel)}
            />
            </>
            )
          ) : (
            <p style={{ color: "var(--muted)" }}>Nenhum equipamento MikroTik cadastrado.</p>
          )}
        </>
      )}

      {realtimeModalOpen && (
        <div style={{ position: "fixed", inset: 0, background: "var(--overlay)", display: "grid", placeItems: "center", zIndex: 40 }} onClick={() => setRealtimeModalOpen(false)}>
          <div className="card" style={{ width: "min(420px, 92vw)" }} onClick={(e) => e.stopPropagation()}>
            <h3 style={{ marginTop: 0 }}>Intervalo tempo real (ms)</h3>
            <input className="input mono" type="number" min={1500} step={500} value={realtimeDraft} onChange={(e) => setRealtimeDraft(e.target.value)} />
            <div className="row" style={{ justifyContent: "flex-end", marginTop: 10, gap: 8 }}>
              <button type="button" className="btn" onClick={() => setRealtimeModalOpen(false)}>
                Cancelar
              </button>
              <button
                type="button"
                className="btn btn--primary"
                onClick={() => {
                  setRealtimeMs(Math.max(1500, Number(realtimeDraft) || 3000));
                  setRealtimeOn(true);
                  setRealtimeModalOpen(false);
                }}
              >
                Iniciar
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

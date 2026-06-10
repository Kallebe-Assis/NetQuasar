import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { PageCountPill } from "../components/PageCountPill";
import { apiFetch } from "../lib/api";
import { EM_DASH, formatDbm } from "../lib/formatDisplay";
import { formatBitrate } from "../lib/formatBitrate";
import { isAdminUser } from "../lib/auth";
import { DropdownMenu } from "../components/DropdownMenu";
import { useAppToast } from "../lib/appToast";
import { toastErr, toastOk } from "../lib/operationToast";
import { collectDeviceTelemetry } from "../lib/telemetryCollectToast";
import { formatCollectedPt, parseMikrotikCollectionStatus } from "../lib/deviceReportHelpers";
import { MikrotikMetricsOverview } from "../components/MikrotikMetricsOverview";

type DeviceRow = {
  id: string;
  description?: string | null;
  category?: string | null;
  brand?: string | null;
  model?: string | null;
  ip?: string | null;
  telemetry_enabled?: boolean;
};

type IfRow = {
  if_index: number;
  descr?: string;
  if_name?: string;
  display_name?: string;
  sfp?: boolean;
  tx_dbm?: number;
  rx_dbm?: number;
  speed_bps?: number;
  admin_status?: string;
  oper_status?: string;
  in_octets?: number;
  out_octets?: number;
  in_bps?: number;
  out_bps?: number;
};

function ifDisplayLabel(r: IfRow): string {
  const s = String(r.display_name ?? r.if_name ?? r.descr ?? "").trim();
  return s || EM_DASH;
}

type SensorRow = { oid?: string; value?: string; type?: string };

function isMikrotik(d: DeviceRow): boolean {
  const c = String(d.category ?? "").toLowerCase();
  const b = String(d.brand ?? "").toLowerCase();
  return c.includes("mikrotik") || b.includes("mikrotik");
}

function inferIfaceType(r: IfRow): string {
  const n = String(r.if_name ?? r.display_name ?? r.descr ?? "").toLowerCase();
  if (n.includes("wlan") || n.includes("wifi")) return "Wireless";
  if (n.includes("sfp")) return "SFP";
  if (n.includes("bridge")) return "Bridge";
  if (n.includes("pppoe")) return "PPPoE";
  if (n.includes("vlan")) return "VLAN";
  return "Ether";
}

function ifaceStatus(r: IfRow): "up" | "down" | "other" {
  const s = String(r.oper_status ?? "").toLowerCase();
  if (s === "up") return "up";
  if (s === "down") return "down";
  return "other";
}

function MiniTrafficChart({
  points,
}: {
  points: Array<{ ts: number; tx: number; rx: number }>;
}) {
  if (points.length < 2) {
    const p = points[0];
    return (
      <div>
        <p style={{ fontSize: 12, color: "var(--muted)", margin: "0 0 6px 0" }}>Aguardando histórico do gráfico em tempo real.</p>
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
  const maxV = Math.max(
    1,
    ...points.map((p) => (Number.isFinite(p.tx) ? p.tx : 0)),
    ...points.map((p) => (Number.isFinite(p.rx) ? p.rx : 0)),
  );
  const xFor = (i: number) => pad + (i * (w - pad * 2)) / Math.max(1, points.length - 1);
  const yFor = (v: number) => h - pad - (Math.max(0, v) / maxV) * (h - pad * 2);
  const txPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.tx)}`).join(" ");
  const rxPath = points.map((p, i) => `${i === 0 ? "M" : "L"} ${xFor(i)} ${yFor(p.rx)}`).join(" ");
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label="Gráfico de tráfego da interface">
      <rect x={0} y={0} width={w} height={h} fill="transparent" />
      <line x1={pad} y1={h - pad} x2={w - pad} y2={h - pad} stroke="var(--border)" strokeWidth={1} />
      <path d={txPath} fill="none" stroke="#f59e0b" strokeWidth={2} />
      <path d={rxPath} fill="none" stroke="#3b82f6" strokeWidth={2} />
    </svg>
  );
}

export function MikrotikPage() {
  const canMutate = isAdminUser();
  const qc = useQueryClient();
  const { push: pushToast, dismiss: dismissToast } = useAppToast();
  const [telCollecting, setTelCollecting] = useState(false);
  const [sel, setSel] = useState<string | null>(null);
  const [tab, setTab] = useState<"overview" | "interfaces">("overview");
  const [realtimeOn, setRealtimeOn] = useState(false);
  const [realtimeMs, setRealtimeMs] = useState(3000);
  const [realtimeModalOpen, setRealtimeModalOpen] = useState(false);
  const [realtimeDraft, setRealtimeDraft] = useState("3000");
  const [liveTable, setLiveTable] = useState<IfRow[]>([]);
  const [selectedChartIfs, setSelectedChartIfs] = useState<number[]>([]);
  const [trafficHistory, setTrafficHistory] = useState<Record<number, Array<{ ts: number; tx: number; rx: number }>>>({});
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"all" | "up" | "down">("all");
  const [typeFilter, setTypeFilter] = useState<"all" | "Ether" | "Wireless" | "SFP" | "Bridge" | "PPPoE" | "VLAN">("all");
  const [trafficFilter, setTrafficFilter] = useState<"all" | "with" | "without">("all");
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

  const iface = useQuery({
    queryKey: ["mikrotik-if", sel],
    enabled: !!sel,
    queryFn: () =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: IfRow[];
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
    queryFn: () => apiFetch<{ collected_at?: string; metrics?: Record<string, unknown> }>(`/api/v1/telemetry/devices/${sel}/latest`),
  });

  const mikrotikConfig = useQuery({
    queryKey: ["mikrotik-collection"],
    queryFn: () =>
      apiFetch<{
        metrics: Record<string, { enabled?: boolean; oid?: string; collect_mode?: string; value_divisor?: number }>;
        catalog: Array<{ key: string; section: string; label: string; unit?: string; default_divisor?: number }>;
        sections: Record<string, string>;
      }>("/api/v1/settings/mikrotik-collection"),
  });

  const refreshIf = useMutation({
    mutationFn: (id: string) =>
      apiFetch<{
        device_id: string;
        collected_at?: string;
        interface_table?: IfRow[];
        interface_count?: number;
        walk_truncated?: boolean;
        walk_note?: string;
        optical_sensors?: SensorRow[];
        note?: string;
      }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} }),
    onSuccess: (data, id) => {
      if (data && data.device_id) {
        qc.setQueryData(["mikrotik-if", id], data);
      }
      qc.invalidateQueries({ queryKey: ["mikrotik-if", id] });
      const n = data.interface_count ?? data.interface_table?.length;
      toastOk(
        pushToast,
        typeof n === "number" ? `Interfaces actualizadas (${n} interface(s)).` : "Interfaces actualizadas com sucesso.",
      );
    },
    onError: (e) => toastErr(pushToast, e, "Falha ao actualizar interfaces."),
  });

  const realtimeTick = useMutation({
    mutationFn: async (id: string) => {
      try {
        return await apiFetch<{
          updates?: Array<{
            if_index: number;
            tx_dbm?: number;
            rx_dbm?: number;
            in_bps?: number;
            out_bps?: number;
          }>;
        }>(`/api/v1/interfaces/devices/${id}/realtime`, { method: "POST", json: {} });
      } catch (e) {
        const msg = String((e as Error)?.message ?? e);
        const isNotFound = msg.includes("404") || msg.toLowerCase().includes("not found");
        if (!isNotFound) throw e;
        // Fallback para backend ainda sem a rota de realtime.
        const full = await apiFetch<{
          interface_table?: IfRow[];
        }>(`/api/v1/interfaces/devices/${id}/refresh`, { method: "POST", json: {} });
        const updates =
          (full.interface_table ?? []).map((r) => ({
              if_index: r.if_index,
              tx_dbm: r.tx_dbm,
              rx_dbm: r.rx_dbm,
              in_bps: r.in_bps,
              out_bps: r.out_bps,
            })) ?? [];
        return { updates };
      }
    },
    onSuccess: (data) => {
      const ups = data?.updates ?? [];
      if (ups.length === 0) return;
      setLiveTable((prev) =>
        prev.map((row) => {
          const up = ups.find((u) => u.if_index === row.if_index);
          if (!up) return row;
          return {
            ...row,
            tx_dbm: up.tx_dbm ?? row.tx_dbm,
            rx_dbm: up.rx_dbm ?? row.rx_dbm,
            in_bps: up.in_bps ?? row.in_bps,
            out_bps: up.out_bps ?? row.out_bps,
          };
        }),
      );
    },
    onError: (e) => toastErr(pushToast, e, "Falha na actualização em tempo real."),
  });

  const runTelCollect = async () => {
    if (!sel || !selectedDevice) return;
    setTelCollecting(true);
    try {
      await collectDeviceTelemetry(sel, selectedDevice.description ?? "", { push: pushToast, dismiss: dismissToast }, qc);
      await qc.invalidateQueries({ queryKey: ["mikrotik-tel", sel] });
    } catch {
      /* toast já exibido */
    } finally {
      setTelCollecting(false);
    }
  };

  const table = liveTable;
  const interfaceTotal = iface.data?.interface_count ?? table.length;
  const walkTruncated = Boolean(iface.data?.walk_truncated) || /truncad/i.test(String(iface.data?.walk_note ?? iface.data?.note ?? ""));
  const collectionStatus = useMemo(
    () => parseMikrotikCollectionStatus(telemetry.data?.metrics as Record<string, unknown> | undefined),
    [telemetry.data?.metrics],
  );

  const onOffSummary = useMemo(() => {
    let up = 0;
    let down = 0;
    for (const r of table) {
      const s = ifaceStatus(r);
      if (s === "up") up += 1;
      if (s === "down") down += 1;
    }
    return { up, down, total: table.length };
  }, [table]);

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
      const hay = `${r.if_index} ${r.display_name ?? ""} ${r.if_name ?? ""} ${r.descr ?? ""} ${type}`.toLowerCase();
      return hay.includes(q);
    });
  }, [table, search, statusFilter, typeFilter, trafficFilter]);

  const selectedChartRows = useMemo(
    () => table.filter((r) => selectedChartIfs.includes(r.if_index)),
    [table, selectedChartIfs],
  );

  useEffect(() => {
    setLiveTable(iface.data?.interface_table ?? []);
  }, [iface.data?.interface_table, sel]);

  useEffect(() => {
    const now = Date.now();
    setTrafficHistory((prev) => {
      const next = { ...prev };
      for (const r of table) {
        const tx = Number(r.out_bps ?? NaN);
        const rx = Number(r.in_bps ?? NaN);
        if (!Number.isFinite(tx) || !Number.isFinite(rx)) continue;
        const arr = [...(next[r.if_index] ?? [])];
        arr.push({ ts: now, tx, rx });
        next[r.if_index] = arr.slice(-50);
      }
      return next;
    });
  }, [table]);

  useEffect(() => {
    if (!realtimeOn || !sel) return;
    const intervalMs = Math.max(1500, Number(realtimeMs) || 3000);
    const timer = window.setInterval(() => {
      if (!realtimeTick.isPending) realtimeTick.mutate(sel);
    }, intervalMs);
    return () => window.clearInterval(timer);
  }, [realtimeOn, realtimeMs, sel, realtimeTick]);

  useEffect(() => {
    setRealtimeOn(false);
    setSelectedChartIfs([]);
  }, [sel]);

  useEffect(() => {
    if (!sel && rows.length > 0) setSel(rows[0].id);
  }, [rows, sel]);

  return (
    <>
      {devices.isLoading && <p>A carregar equipamentos…</p>}
      {devices.isError && <div className="msg msg--err">{(devices.error as Error).message}</div>}
      {devices.isLoading || devices.isError ? null : (
        <>
          <style>{`
            .mk-switch {
              position: relative;
              width: 36px;
              height: 20px;
              border-radius: 999px;
              border: 1px solid var(--border);
              background: rgba(255,255,255,0.12);
              transition: background 140ms ease;
              display: inline-block;
              vertical-align: middle;
            }
            .mk-switch__knob {
              position: absolute;
              top: 1px;
              left: 1px;
              width: 16px;
              height: 16px;
              border-radius: 999px;
              background: #fff;
              transition: transform 140ms ease;
            }
            .mk-switch--on {
              background: rgba(34,197,94,0.45);
              border-color: rgba(34,197,94,0.7);
            }
            .mk-switch--on .mk-switch__knob {
              transform: translateX(16px);
            }
            .mk-options-item {
              display: flex;
              align-items: center;
              justify-content: space-between;
              gap: 8px;
              width: 100%;
              padding: 8px 10px;
              border-radius: 6px;
              color: inherit;
              text-decoration: none;
              background: transparent;
            }
            .mk-options-item:hover { background: var(--hover-bg-menu); }
          `}</style>
          <div className="page-heading">
            <h1>MikroTik</h1>
            <PageCountPill label="Mikrotiks" count={rows.length} />
          </div>
          <div className="row" style={{ gap: 8, marginBottom: 12 }}>
            <button type="button" className={tab === "overview" ? "btn btn--primary" : "btn"} onClick={() => setTab("overview")}>
              Overview
            </button>
            <button type="button" className={tab === "interfaces" ? "btn btn--primary" : "btn"} onClick={() => setTab("interfaces")}>
              Interfaces
            </button>
          </div>

          {tab === "overview" && (
            <div className="card" style={{ minWidth: 0, maxWidth: "100%" }}>
              <div className="row" style={{ gap: 8, alignItems: "center", marginBottom: 10 }}>
                <label className="mono" style={{ fontSize: 12 }}>MikroTik:</label>
                <select className="input" style={{ maxWidth: 420 }} value={sel ?? ""} onChange={(e) => setSel(e.target.value || null)}>
                  {rows.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.description ?? "—"}
                    </option>
                  ))}
                </select>
                {canMutate ? (
                  <button type="button" className="btn" disabled={telCollecting || !sel} onClick={() => void runTelCollect()}>
                    {telCollecting ? "Coletando..." : "Atualizar telemetria"}
                  </button>
                ) : null}
              </div>
              {!selectedDevice ? (
                <p style={{ color: "var(--muted)" }}>Selecione um MikroTik.</p>
              ) : (
                <>
                  <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                    Últ. interfaces: <span className="mono">{formatCollectedPt(iface.data?.collected_at)}</span> · Últ. telemetria:{" "}
                    <span className="mono">{formatCollectedPt(telemetry.data?.collected_at)}</span>
                  </p>
                  {collectionStatus && (collectionStatus.missingOid.length > 0 || collectionStatus.message) ? (
                    <div className="msg msg--warn" style={{ fontSize: 12, marginBottom: 10 }}>
                      {collectionStatus.message ||
                        "Algumas métricas activas no perfil não têm OID configurado e não foram colectadas."}
                      {collectionStatus.missingOid.length > 0 ? (
                        <span>
                          {" "}
                          Campos:{" "}
                          <span className="mono">{collectionStatus.missingOid.join(", ")}</span>
                        </span>
                      ) : null}
                    </div>
                  ) : null}
                  <MikrotikMetricsOverview
                    metrics={telemetry.data?.metrics as Record<string, unknown> | undefined}
                    catalog={mikrotikConfig.data?.catalog ?? []}
                    config={mikrotikConfig.data?.metrics ?? {}}
                    sectionLabels={mikrotikConfig.data?.sections ?? {}}
                    deviceLabel={selectedDevice.description ?? "—"}
                    deviceModel={selectedDevice.model}
                    deviceIp={selectedDevice.ip}
                    collectedAt={telemetry.data?.collected_at}
                    formatCollectedAt={formatCollectedPt}
                    ifaceUp={onOffSummary.up}
                    ifaceDown={onOffSummary.down}
                    ifaceTotal={onOffSummary.total}
                  />
                </>
              )}
            </div>
          )}

          {tab === "interfaces" && (
            <div className="card" style={{ minWidth: 0, maxWidth: "100%", overflowX: "hidden" }}>
              <div className="row" style={{ justifyContent: "space-between", alignItems: "center", marginBottom: 8 }}>
                <h2 style={{ margin: 0 }}>Interfaces</h2>
                <DropdownMenu
                  key={sel ?? "mk-options"}
                  align="end"
                  minWidth={210}
                  trigger={({ toggle, open }) => (
                    <button type="button" className="btn" onClick={toggle} title="Opções" aria-haspopup="menu" aria-expanded={open}>
                      ⚙
                    </button>
                  )}
                >
                  {({ close }) => (
                    <Link to="/devices" className="mk-options-item" onClick={() => close()}>
                      <span>Ir para equipamentos</span>
                      <span aria-hidden style={{ opacity: 0.7 }}>↗</span>
                    </Link>
                  )}
                </DropdownMenu>
              </div>
              <div className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap", marginBottom: 12 }}>
                <select className="input" style={{ minWidth: 260 }} value={sel ?? ""} onChange={(e) => setSel(e.target.value || null)}>
                  {rows.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.description ?? "—"} {d.ip ? `(${d.ip})` : ""}
                    </option>
                  ))}
                </select>
                <input className="input" style={{ minWidth: 220 }} placeholder="Buscar interface..." value={search} onChange={(e) => setSearch(e.target.value)} />
                <select className="input" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value as "all" | "up" | "down")}>
                  <option value="all">Todos status</option>
                  <option value="up">UP</option>
                  <option value="down">DOWN</option>
                </select>
                <select className="input" value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as "all" | "Ether" | "Wireless" | "SFP" | "Bridge" | "PPPoE" | "VLAN")}>
                  <option value="all">Todos tipos</option>
                  <option value="Ether">Ether</option>
                  <option value="Wireless">Wireless</option>
                  <option value="SFP">SFP</option>
                  <option value="Bridge">Bridge</option>
                  <option value="PPPoE">PPPoE</option>
                  <option value="VLAN">VLAN</option>
                </select>
                <select className="input" value={trafficFilter} onChange={(e) => setTrafficFilter(e.target.value as "all" | "with" | "without")}>
                  <option value="all">Todo tráfego</option>
                  <option value="with">Com tráfego</option>
                  <option value="without">Sem tráfego</option>
                </select>
                {canMutate ? (
                  <>
                    <button type="button" className="btn btn--primary" disabled={refreshIf.isPending || !sel} onClick={() => sel && refreshIf.mutate(sel)}>
                      {refreshIf.isPending ? "Atualizando..." : "Atualizar"}
                    </button>
                    <button
                      type="button"
                      className={realtimeOn ? "btn btn--danger" : "btn"}
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
              </div>
              {selectedChartIfs.length > 0 && (
                <div className="row" style={{ marginTop: -2, marginBottom: 8 }}>
                  <button
                    type="button"
                    className="btn"
                    style={{ fontSize: 11, opacity: 0.9 }}
                    onClick={() => chartsRef.current?.scrollIntoView({ behavior: "smooth", block: "start" })}
                  >
                    ↓ Ver gráficos
                  </button>
                </div>
              )}
              <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 0 }}>
                Últ. interfaces: <span className="mono">{formatCollectedPt(iface.data?.collected_at)}</span> · Últ. telemetria:{" "}
                <span className="mono">{formatCollectedPt(telemetry.data?.collected_at)}</span>
                {" · "}
                <strong>{interfaceTotal}</strong> interface{interfaceTotal === 1 ? "" : "s"} no snapshot
                {interfaceRowsFiltered.length !== table.length
                  ? ` · a mostrar ${interfaceRowsFiltered.length} com filtros`
                  : " · todas listadas abaixo"}
              </p>
              {walkTruncated && (
                <div className="msg" style={{ fontSize: 12, marginBottom: 8, borderColor: "var(--warn)", color: "var(--text)" }}>
                  A coleta SNMP foi truncada antes de obter todas as interfaces. Aumente o timeout de snapshot de interfaces em Configurações →
                  Alertas e execute <strong>Atualizar</strong> novamente.
                  {iface.data?.walk_note ? <span className="mono" style={{ display: "block", marginTop: 4, fontSize: 10 }}>{iface.data.walk_note}</span> : null}
                </div>
              )}
              {(iface.isLoading || refreshIf.isPending || (realtimeOn && realtimeTick.isPending)) && (
                <p style={{ fontSize: 11, color: "var(--muted)" }}>Coletando dados de interface...</p>
              )}
              {realtimeOn && <p style={{ fontSize: 11, color: "var(--ok)" }}>Monitoramento em tempo real ativo ({Math.max(1500, realtimeMs)} ms) para tráfego e potência óptica SFP.</p>}
              {refreshIf.isError && <div className="msg msg--err">{(refreshIf.error as Error).message}</div>}
              {realtimeTick.isError && <div className="msg msg--err">{(realtimeTick.error as Error).message}</div>}

              <div className="table-wrap" style={{ maxHeight: "min(75vh, 720px)", overflowY: "auto", overflowX: "auto", maxWidth: "100%" }}>
                <table style={{ fontSize: 9, width: "100%", tableLayout: "fixed" }}>
                  <thead>
                    <tr>
                      <th style={{ width: 36 }}>Idx</th>
                      <th style={{ width: "26%" }}>Nome</th>
                      <th style={{ width: 64 }}>Tipo</th>
                      <th style={{ width: 58 }}>Status</th>
                      <th className="mono" style={{ width: 78 }}>TX tráfego</th>
                      <th className="mono" style={{ width: 78 }}>RX tráfego</th>
                      <th className="mono" style={{ width: 74 }}>TX dBm</th>
                      <th className="mono" style={{ width: 74 }}>RX dBm</th>
                      <th style={{ width: 66 }}>Vel.</th>
                      <th style={{ width: 88 }}>Exibir gráfico</th>
                    </tr>
                  </thead>
                  <tbody>
                    {interfaceRowsFiltered.map((r) => (
                      <tr key={r.if_index}>
                        <td className="mono">{r.if_index}</td>
                        <td style={{ wordBreak: "break-word", overflowWrap: "anywhere" }}>{ifDisplayLabel(r)}</td>
                        <td>{inferIfaceType(r)}</td>
                        <td>
                          <span className={ifaceStatus(r) === "up" ? "badge badge--ok" : ifaceStatus(r) === "down" ? "badge badge--err" : "badge badge--off"} style={{ fontSize: 9, lineHeight: 1.1, padding: "1px 5px" }}>
                            {ifaceStatus(r).toUpperCase()}
                          </span>
                        </td>
                        <td className="mono">{formatBitrate(r.out_bps)}</td>
                        <td className="mono">{formatBitrate(r.in_bps)}</td>
                        <td className="mono">{formatDbm(r.tx_dbm)}</td>
                        <td className="mono">{formatDbm(r.rx_dbm)}</td>
                        <td className="mono">{r.speed_bps != null && r.speed_bps > 0 ? formatBitrate(r.speed_bps) : "—"}</td>
                        <td>
                          <label
                            style={{
                              display: "inline-flex",
                              alignItems: "center",
                              gap: 6,
                              cursor: "pointer",
                              userSelect: "none",
                            }}
                            title="Ativar/desativar gráfico individual da interface"
                          >
                            <input
                              type="checkbox"
                              checked={selectedChartIfs.includes(r.if_index)}
                              onChange={(e) => {
                                const now = Date.now();
                                if (e.target.checked) {
                                  setTrafficHistory((prev) => {
                                    const arr = [...(prev[r.if_index] ?? [])];
                                    const tx = Number(r.out_bps ?? 0);
                                    const rx = Number(r.in_bps ?? 0);
                                    if (Number.isFinite(tx) && Number.isFinite(rx)) {
                                      arr.push({ ts: now, tx, rx });
                                    }
                                    return { ...prev, [r.if_index]: arr.slice(-50) };
                                  });
                                  setSelectedChartIfs((prev) => (prev.includes(r.if_index) ? prev : [...prev, r.if_index]));
                                  return;
                                }
                                setSelectedChartIfs((prev) => prev.filter((x) => x !== r.if_index));
                              }}
                              style={{ display: "none" }}
                            />
                            <span className={`mk-switch ${selectedChartIfs.includes(r.if_index) ? "mk-switch--on" : ""}`} aria-hidden>
                              <span className="mk-switch__knob" />
                            </span>
                            <span className="mono" style={{ fontSize: 9 }}>
                              {selectedChartIfs.includes(r.if_index) ? "ON" : "OFF"}
                            </span>
                          </label>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              {interfaceRowsFiltered.length === 0 && !iface.isLoading && (
                <p style={{ color: "var(--muted)", fontSize: 12 }}>
                  {table.length === 0 ? "Nenhuma interface no snapshot — use Atualizar para coletar via SNMP." : "Nenhuma interface encontrada para os filtros."}
                </p>
              )}
              {selectedChartRows.length > 0 && (
                <div ref={chartsRef} style={{ marginTop: 10 }}>
                  <div className="row" style={{ justifyContent: "space-between", marginBottom: 8 }}>
                    <strong>Gráficos de interfaces selecionadas</strong>
                    <button type="button" className="btn" onClick={() => setSelectedChartIfs([])}>Limpar seleção</button>
                  </div>
                  <div
                    style={{
                      display: "grid",
                      gridTemplateColumns: "repeat(3, minmax(0, 1fr))",
                      gap: 10,
                    }}
                  >
                    {selectedChartRows.map((row) => (
                      <div key={`chart-${row.if_index}`} className="card" style={{ minWidth: 0 }}>
                        <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
                          <strong style={{ fontSize: 12 }}>{ifDisplayLabel(row)}</strong>
                          <button type="button" className="btn" style={{ padding: "2px 6px", minHeight: 0 }} onClick={() => setSelectedChartIfs((prev) => prev.filter((x) => x !== row.if_index))}>
                            OFF
                          </button>
                        </div>
                        <MiniTrafficChart points={trafficHistory[row.if_index] ?? []} />
                      </div>
                    ))}
                  </div>
                  <p style={{ fontSize: 11, color: "var(--muted)", marginTop: 6, marginBottom: 0 }}>
                    Linha laranja = TX tráfego · Linha azul = RX tráfego
                  </p>
                </div>
              )}
            </div>
          )}
          {realtimeModalOpen && (
            <div
              style={{
                position: "fixed",
                inset: 0,
                background: "rgba(0,0,0,0.45)",
                display: "grid",
                placeItems: "center",
                zIndex: 30,
              }}
              onClick={() => setRealtimeModalOpen(false)}
            >
              <div className="card" style={{ width: "min(420px, 92vw)" }} onClick={(e) => e.stopPropagation()}>
                <h3 style={{ marginTop: 0 }}>Configurar tempo real</h3>
                <p style={{ fontSize: 12, color: "var(--muted)" }}>Informe o intervalo de atualização em milissegundos (mínimo 1500 ms).</p>
                <input
                  className="input mono"
                  type="number"
                  min={1500}
                  step={500}
                  value={realtimeDraft}
                  onChange={(e) => setRealtimeDraft(e.target.value)}
                />
                <div className="row" style={{ justifyContent: "flex-end", marginTop: 10, gap: 8 }}>
                  <button type="button" className="btn" onClick={() => setRealtimeModalOpen(false)}>Cancelar</button>
                  <button
                    type="button"
                    className="btn btn--primary"
                    onClick={() => {
                      const n = Number(realtimeDraft);
                      const ms = Number.isFinite(n) ? Math.max(1500, Math.round(n)) : 3000;
                      setRealtimeMs(ms);
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
      )}
    </>
  );
}
